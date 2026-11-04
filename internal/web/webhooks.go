package web

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/telephony"
	"github.com/vivekyarra/bhagirath/internal/whatsapp"
)

// ---- WhatsApp Cloud API ----

func (s *Server) whatsappVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tok := s.Cfg.WhatsApp.VerifyToken
	if q.Get("hub.mode") != "subscribe" || tok == "" || subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(tok)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_, _ = io.WriteString(w, q.Get("hub.challenge"))
}

func (s *Server) whatsappWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if s.Cfg.WhatsApp.AppSecret == "" || !whatsapp.VerifySignature(s.Cfg.WhatsApp.AppSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	msgs, statuses, err := whatsapp.Parse(body)
	if err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	for _, st := range statuses {
		if st.Status == "delivered" || st.Status == "read" {
			_, _ = s.Engine.Store.MarkDeliveredByProvider(r.Context(), st.ID)
		}
	}
	for _, m := range msgs {
		m := m
		if !s.enqueue(func(ctx context.Context) { s.processWhatsApp(ctx, m) }) {
			// Queue full: ask Meta to retry later rather than drop the message.
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) processWhatsApp(ctx context.Context, in whatsapp.Inbound) {
	m := agents.Message{ProviderMsgID: in.ID, Phone: "+" + in.From, Text: in.Text, ReplyID: in.ReplyID}
	switch in.Type {
	case "location":
		m.Point = &domain.Point{Lat: in.Lat, Lon: in.Lon}
	case "audio":
		if s.WhatsApp == nil {
			return
		}
		data, ct, err := s.WhatsApp.GetMedia(ctx, in.MediaID)
		if err != nil {
			s.Log.Error("whatsapp media download failed", "err", err)
			return
		}
		m.Audio, m.AudioType = data, ct
	case "image", "other":
		if in.Text == "" {
			return
		}
	}
	if _, err := s.Engine.HandleMessage(ctx, m); err != nil {
		s.Log.Error("whatsapp message failed", "err", err)
		return
	}
	_, _ = s.Dispatcher.Run(ctx)
}

// ---- Twilio Programmable Voice ----

// twilioOK validates X-Twilio-Signature against the public URL.
func (s *Server) twilioOK(r *http.Request) bool {
	if err := r.ParseForm(); err != nil || s.Twilio == nil {
		return false
	}
	full := s.Cfg.BaseURL + r.URL.RequestURI()
	return telephony.ValidateTwilioSignature(s.Cfg.Twilio.AuthToken, full, r.PostForm, r.Header.Get("X-Twilio-Signature"))
}

func twiml(w http.ResponseWriter, t *telephony.TwiML) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = io.WriteString(w, t.String())
}

func say(l, text string) telephony.Say {
	v := i18n.TwilioVoice[l]
	if v[0] == "" {
		v = i18n.TwilioVoice["hi"]
	}
	return telephony.Say{Voice: v[0], Language: v[1], Text: text}
}

// residentNumber returns the resident's number for inbound or outbound calls.
func residentNumber(f url.Values) string {
	if strings.HasPrefix(f.Get("Direction"), "outbound") {
		return f.Get("To")
	}
	return f.Get("From")
}

// twilioIncoming handles a call to the Bhagirath number: it is rejected
// (so the caller pays nothing — a "missed call") and Bhagirath calls back.
func (s *Server) twilioIncoming(w http.ResponseWriter, r *http.Request) {
	if !s.twilioOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	from := r.PostForm.Get("From")
	twiml(w, (&telephony.TwiML{}).Add(telephony.Reject{Reason: "busy"}))
	phone, err := secure.NormalizePhone(from)
	if err != nil {
		return
	}
	// One callback per number per 10 minutes, so repeated rings do not cost money.
	key := "missed:" + s.Engine.Vault.HashPhone(phone)
	now := s.Engine.Clock.Now()
	if v, err := s.Engine.Store.KVGet(r.Context(), key); err == nil {
		if t, err := time.Parse(time.RFC3339, v); err == nil && now.Sub(t) < 10*time.Minute {
			return
		}
	}
	_ = s.Engine.Store.KVSet(r.Context(), key, now.Format(time.RFC3339), now)
	s.enqueue(func(ctx context.Context) {
		if _, err := s.Twilio.Call(ctx, phone, "", s.Cfg.BaseURL+"/voice/twilio/ivr?step=lang", s.Cfg.BaseURL+"/voice/twilio/status"); err != nil {
			s.Log.Error("missed-call callback failed", "err", err)
		}
	})
}

var ivrLangs = []string{"hi", "en", "mr", "te"}

var langPrompt = map[string]string{
	"hi": "हिंदी के लिए 1 दबाएँ।", "en": "For English, press 2.", "mr": "मराठीसाठी 3 दाबा.", "te": "తెలుగు కోసం 4 నొక్కండి.",
}

// twilioIVR runs the complaint IVR: language → consent → record → thanks.
func (s *Server) twilioIVR(w http.ResponseWriter, r *http.Request) {
	if !s.twilioOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	l := q.Get("lang")
	if !lang.IsSupported(l) {
		l = "hi"
	}
	base := s.Cfg.BaseURL + "/voice/twilio/ivr"
	doc := &telephony.TwiML{}
	switch q.Get("step") {
	case "lang":
		g := telephony.Gather{Action: base + "?step=consent", Method: "POST", NumDigits: 1, Timeout: 6}
		for _, x := range ivrLangs {
			g.Says = append(g.Says, say(x, langPrompt[x]))
		}
		doc.Add(say("hi", i18n.T("ivr_welcome", "hi", nil)), g, telephony.Redirect{Method: "POST", URL: base + "?step=consent&lang=hi"})
	case "consent":
		if d := telephony.Digits(r.PostForm.Get("Digits")); d >= 1 && d <= len(ivrLangs) {
			l = ivrLangs[d-1]
		}
		g := telephony.Gather{Action: base + "?step=record&lang=" + l, Method: "POST", NumDigits: 1, Timeout: 8,
			Says: []telephony.Say{say(l, i18n.T("ivr_consent", l, nil))}}
		doc.Add(say(l, i18n.T("ivr_welcome", l, nil)), g, telephony.Redirect{Method: "POST", URL: base + "?step=record&lang=" + l + "&noconsent=1"})
	case "record":
		consent := "0"
		if r.PostForm.Get("Digits") == "1" && q.Get("noconsent") == "" {
			consent = "1"
		}
		doc.Add(say(l, i18n.T("ivr_record", l, nil)),
			telephony.Record{Action: base + "?step=recorded&lang=" + l + "&consent=" + consent, Method: "POST", MaxLength: 120, FinishOnKey: "#", PlayBeep: true, Timeout: 6})
	case "recorded":
		doc.Add(say(l, i18n.T("ivr_thanks", l, nil)), telephony.Hangup{})
		recURL, callSID := r.PostForm.Get("RecordingUrl"), r.PostForm.Get("CallSid")
		phone := residentNumber(r.PostForm)
		consent := q.Get("consent") == "1"
		channel := domain.ChannelMissedCall
		if !strings.HasPrefix(r.PostForm.Get("Direction"), "outbound") {
			channel = domain.ChannelIVR
		}
		if recURL != "" {
			s.enqueue(func(ctx context.Context) {
				data, ct, err := s.Twilio.FetchRecording(ctx, recURL)
				if err != nil {
					s.Log.Error("recording download failed", "err", err)
					return
				}
				if _, err := s.Engine.Intake(ctx, agents.Inbound{Channel: channel, ProviderMsgID: callSID, Phone: phone, Audio: data, AudioType: ct,
					Language: l, Consent: &consent, Actor: "resident:ivr"}); err != nil {
					s.Log.Error("ivr intake failed", "err", err)
				}
				_, _ = s.Dispatcher.Run(ctx)
			})
		}
	default:
		doc.Add(telephony.Hangup{})
	}
	twiml(w, doc)
}

// twilioCallbackAnswer records the keypress on a fix-verification call.
func (s *Server) twilioCallbackAnswer(w http.ResponseWriter, r *http.Request) {
	if !s.twilioOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("closure"), 10, 64)
	d := telephony.Digits(r.PostForm.Get("Digits"))
	l := "hi"
	if err == nil && (d == 1 || d == 2) {
		if cl, err := s.Engine.Store.GetClosure(r.Context(), id); err == nil {
			if c, err := s.Engine.Store.GetComplaint(r.Context(), cl.ComplaintID); err == nil && c.ReporterID != nil {
				if rep, err := s.Engine.Store.GetReporter(r.Context(), *c.ReporterID); err == nil {
					l = lang.Pick(rep.PreferredLanguage, "hi")
					// Only the complainant's own phone may answer.
					if pn, err := secure.NormalizePhone(residentNumber(r.PostForm)); err == nil && s.Engine.Vault.HashPhone(pn) == rep.PhoneHash {
						_ = s.Engine.RecordCallback(r.Context(), id, d == 1, "voice", "resident:"+rep.Ref)
					}
				}
			}
		}
	}
	twiml(w, (&telephony.TwiML{}).Add(say(l, i18n.T("ivr_thanks", l, nil)), telephony.Hangup{}))
	s.kickDispatcher()
}

func (s *Server) twilioStatus(w http.ResponseWriter, r *http.Request) {
	if !s.twilioOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if sid := r.PostForm.Get("CallSid"); sid != "" && r.PostForm.Get("CallStatus") == "completed" {
		_, _ = s.Engine.Store.MarkDeliveredByProvider(r.Context(), sid)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Exotel Passthru applets (GET) ----
//
// Configure an Exotel flow: Passthru(/voice/exotel/missed) on the inbound
// ExoPhone; for the callback flow: Gather → Passthru(/voice/exotel/consent),
// Record → Passthru(/voice/exotel/recording); for verification calls:
// Gather → Passthru(/voice/exotel/callback). Every URL carries ?key=<EXOTEL_WEBHOOK_KEY>.
func (s *Server) exotel(w http.ResponseWriter, r *http.Request) {
	key := s.Cfg.Exotel.WebhookKey
	if key == "" || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(key)) != 1 || s.Exotel == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p := telephony.ParseExotelPassthru(r.URL.Query())
	ctx := r.Context()
	switch r.PathValue("step") {
	case "missed":
		phone, err := secure.NormalizePhone(p.From)
		if err == nil {
			s.enqueue(func(ctx context.Context) {
				if _, err := s.Exotel.Connect(ctx, phone, "ivr", ""); err != nil {
					s.Log.Error("exotel callback failed", "err", err)
				}
			})
		}
	case "consent":
		_ = s.Engine.Store.KVSet(ctx, "exotel-consent:"+p.CallSid, p.Digits, s.Engine.Clock.Now())
	case "recording":
		consent := false
		if v, err := s.Engine.Store.KVGet(ctx, "exotel-consent:"+p.CallSid); err == nil {
			consent = v == "1"
		}
		u, err := url.Parse(p.RecordingURL)
		if err != nil || u.Scheme != "https" || !(strings.HasSuffix(u.Host, ".exotel.com") || strings.HasSuffix(u.Host, ".amazonaws.com") || strings.HasSuffix(u.Host, ".exotel.in")) {
			http.Error(w, "unexpected recording host", http.StatusBadRequest)
			return
		}
		phone := p.From
		s.enqueue(func(ctx context.Context) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			req.SetBasicAuth(s.Cfg.Exotel.APIKey, s.Cfg.Exotel.APIToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				s.Log.Error("exotel recording download", "err", err)
				return
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
			if _, err := s.Engine.Intake(ctx, agents.Inbound{Channel: domain.ChannelMissedCall, ProviderMsgID: p.CallSid, Phone: phone, Audio: data,
				AudioType: resp.Header.Get("Content-Type"), Consent: &consent, Actor: "resident:ivr"}); err != nil {
				s.Log.Error("exotel intake failed", "err", err)
			}
			_, _ = s.Dispatcher.Run(ctx)
		})
	case "callback":
		var id int64
		if _, err := fmt.Sscanf(p.CustomField, "closure:%d", &id); err == nil && (p.Digits == "1" || p.Digits == "2") {
			_ = s.Engine.RecordCallback(ctx, id, p.Digits == "1", "voice", "resident:exotel")
		}
	case "status":
	default:
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}
