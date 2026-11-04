package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/metrics"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

func (s *Server) landing(w http.ResponseWriter, r *http.Request) {
	m, _ := s.Engine.Store.ComputeMetrics(r.Context(), !s.Cfg.Demo(), s.Engine.Clock.Now())
	s.render(w, r, "landing", map[string]any{"Title": "Bhagirath — clean water accountability", "M": m, "Rule": s.Engine.Rule, "FrozenTag": rule.FrozenTag})
}

// Web form labels (the form itself is multilingual).
var formText = map[string]map[string]string{
	"heading":     {"en": "Report a drinking-water problem", "hi": "पीने के पानी की समस्या बताएँ", "mr": "पिण्याच्या पाण्याची समस्या कळवा", "te": "తాగునీటి సమస్యను తెలియజేయండి"},
	"what":        {"en": "What is the problem?", "hi": "समस्या क्या है?", "mr": "समस्या काय आहे?", "te": "సమస్య ఏమిటి?"},
	"describe":    {"en": "Describe it in your own words (any language)", "hi": "अपने शब्दों में बताइए (किसी भी भाषा में)", "mr": "तुमच्या शब्दांत सांगा (कोणत्याही भाषेत)", "te": "మీ మాటల్లో చెప్పండి (ఏ భాషలోనైనా)"},
	"where":       {"en": "Where? A nearby landmark (temple, school, shop)", "hi": "कहाँ? पास की कोई जगह (मंदिर, स्कूल, दुकान)", "mr": "कुठे? जवळचे ठिकाण (मंदिर, शाळा, दुकान)", "te": "ఎక్కడ? దగ్గర్లోని గుర్తు (గుడి, బడి, దుకాణం)"},
	"ward":        {"en": "Ward / village", "hi": "वार्ड / गाँव", "mr": "वॉर्ड / गाव", "te": "వార్డు / గ్రామం"},
	"gps":         {"en": "Use my current location", "hi": "मेरी अभी की लोकेशन लें", "mr": "माझे सध्याचे लोकेशन घ्या", "te": "నా ప్రస్తుత స్థానాన్ని వాడండి"},
	"phone":       {"en": "Mobile number (optional — only to check later that it was fixed)", "hi": "मोबाइल नंबर (वैकल्पिक — सिर्फ़ बाद में पूछने के लिए कि ठीक हुआ या नहीं)", "mr": "मोबाइल नंबर (ऐच्छिक — दुरुस्त झाले का हे नंतर विचारण्यासाठीच)", "te": "మొబైల్ నంబర్ (ఐచ్ఛికం — తర్వాత పరిష్కారమైందా అని అడగడానికి మాత్రమే)"},
	"consent":     {"en": "Bhagirath may call or message me about this complaint. My number stays private.", "hi": "भागीरथ इस शिकायत के बारे में मुझे कॉल/संदेश कर सकता है। मेरा नंबर गोपनीय रहेगा।", "mr": "भगीरथ या तक्रारीबद्दल मला कॉल/संदेश करू शकते. माझा नंबर गोपनीय राहील.", "te": "ఈ ఫిర్యాదు గురించి భగీరథ నన్ను సంప్రదించవచ్చు. నా నంబర్ గోప్యంగా ఉంటుంది."},
	"submit":      {"en": "Send complaint", "hi": "शिकायत भेजें", "mr": "तक्रार पाठवा", "te": "ఫిర్యాదు పంపండి"},
	"done":        {"en": "Your complaint is recorded. Keep this number:", "hi": "आपकी शिकायत दर्ज हो गई है। यह नंबर संभालकर रखें:", "mr": "तुमची तक्रार नोंदवली आहे. हा क्रमांक जपून ठेवा:", "te": "మీ ఫిర్యాదు నమోదైంది. ఈ నంబర్\u200cను గుర్తుంచుకోండి:"},
	"nophone":     {"en": "No smartphone? Give a missed call to the Bhagirath number, or tell a volunteer.", "hi": "स्मार्टफ़ोन नहीं है? भागीरथ नंबर पर मिस्ड कॉल दें, या किसी स्वयंसेवक को बताएँ।", "mr": "स्मार्टफोन नाही? भगीरथ नंबरवर मिस्ड कॉल द्या, किंवा स्वयंसेवकाला सांगा.", "te": "స్మార్ట్\u200cఫోన్ లేదా? భగీరథ నంబర్\u200cకు మిస్డ్ కాల్ ఇవ్వండి, లేదా వాలంటీర్\u200cకు చెప్పండి."},
	"notgov":      {"en": "Bhagirath is a citizen tool, not a government service.", "hi": "भागीरथ नागरिकों का साधन है, सरकारी सेवा नहीं।", "mr": "भगीरथ हे नागरिकांचे साधन आहे, सरकारी सेवा नाही.", "te": "భగీరథ పౌరుల సాధనం, ప్రభుత్వ సేవ కాదు."},
	"choose":      {"en": "— choose —", "hi": "— चुनें —", "mr": "— निवडा —", "te": "— ఎంచుకోండి —"},
	"unknownward": {"en": "I don't know", "hi": "पता नहीं", "mr": "माहीत नाही", "te": "తెలియదు"},
}

func ft(l string) map[string]string {
	out := map[string]string{}
	for k, v := range formText {
		if s, ok := v[l]; ok {
			out[k] = s
		} else {
			out[k] = v["en"]
		}
	}
	return out
}

var formIssues = []domain.IssueType{domain.IssueDirtyWater, domain.IssueFoulSmell, domain.IssueSewageMixing, domain.IssueNoWater, domain.IssueLeak, domain.IssueIllness}

func (s *Server) reportForm(w http.ResponseWriter, r *http.Request) {
	l := pickLang(r)
	if q := r.URL.Query().Get("lang"); lang.IsSupported(q) {
		http.SetCookie(w, &http.Cookie{Name: "bh_lang", Value: q, Path: "/", MaxAge: 365 * 86400, SameSite: http.SameSiteLaxMode})
	}
	wards, _ := s.Engine.Store.ListWards(r.Context())
	s.render(w, r, "report", map[string]any{"Title": ft(l)["heading"], "T": ft(l), "L": l, "Issues": formIssues, "Wards": wards, "CSRF": s.publicCSRF("report")})
}

func (s *Server) reportSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	l := lang.Pick(r.FormValue("lang"), pickLang(r))
	wards, _ := s.Engine.Store.ListWards(r.Context())
	fail := func(msg string) {
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, "report", map[string]any{"Title": ft(l)["heading"], "T": ft(l), "L": l, "Issues": formIssues, "Wards": wards, "Error": msg, "CSRF": s.publicCSRF("report")})
	}
	if !s.validPublicCSRF("report", r.FormValue("csrf")) {
		fail("The form expired. Please submit again.")
		return
	}
	if r.FormValue("website") != "" { // honeypot field, hidden from humans
		http.Redirect(w, r, "/report", http.StatusSeeOther)
		return
	}
	issue := domain.IssueType(r.FormValue("issue"))
	if issue != "" && !domain.Contains(domain.IssueTypes, issue) {
		fail("Unknown problem type.")
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if len(text) > 2000 {
		fail("Description is too long (2000 characters max).")
		return
	}
	if text == "" && issue == "" {
		fail("Please choose a problem or describe it.")
		return
	}
	if text == "" {
		text = i18n.Issue(string(issue), l)
	}
	in := agents.Inbound{Channel: domain.ChannelWeb, Text: text, LocationText: strings.TrimSpace(r.FormValue("landmark")), IssueHint: issue,
		WardHint: r.FormValue("ward"), Language: l, Actor: "resident:web"}
	if lat, err1 := strconv.ParseFloat(r.FormValue("lat"), 64); err1 == nil {
		if lon, err2 := strconv.ParseFloat(r.FormValue("lon"), 64); err2 == nil {
			p := domain.Point{Lat: lat, Lon: lon}
			if p.Valid() {
				in.Point = &p
			}
		}
	}
	if ph := strings.TrimSpace(r.FormValue("phone")); ph != "" {
		if _, err := secure.NormalizePhone(ph); err != nil {
			fail("That mobile number does not look right.")
			return
		}
		in.Phone = ph
		consent := r.FormValue("consent") == "on"
		in.Consent = &consent
		yes := true
		in.HasSmartphone = &yes
	}
	res, err := s.Engine.Intake(r.Context(), in)
	if err != nil {
		s.Log.Error("web intake", "err", err)
		fail("Sorry, something went wrong. Please try again.")
		return
	}
	s.kickDispatcher()
	s.render(w, r, "report_done", map[string]any{"Title": ft(l)["heading"], "T": ft(l), "L": l, "C": res.Complaint, "Located": res.Complaint.WardID != "",
		"Ask": i18n.T("ask_location", l, i18n.Vars{"ref": res.Complaint.Ref})})
}

// kickDispatcher sends queued messages promptly instead of waiting for the tick.
func (s *Server) kickDispatcher() {
	s.enqueue(func(ctx contextT) { _, _ = s.Dispatcher.Run(ctx) })
}

func (s *Server) publicMetrics(r *http.Request) (store.Metrics, []metrics.Target, error) {
	m, err := s.Engine.Store.ComputeMetrics(r.Context(), !s.Cfg.Demo(), s.Engine.Clock.Now())
	if err != nil {
		return m, nil, err
	}
	asrN, wer := s.asrSummary(r)
	return m, metrics.Targets(m, asrN, wer), nil
}

type wardStat struct {
	Ward          domain.Ward
	Level         domain.Level
	Complaints    int
	Verified      int
	MedianAckHrs  *float64
	MedianFixDays *float64
}

func (s *Server) impact(w http.ResponseWriter, r *http.Request) {
	m, ts, err := s.publicMetrics(r)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	stats, _ := s.wardStats(r)
	s.render(w, r, "impact", map[string]any{"Title": "Public impact", "M": m, "Targets": ts, "Wards": stats, "Rule": s.Engine.Rule, "FrozenTag": rule.FrozenTag})
}

func (s *Server) impactJSON(w http.ResponseWriter, r *http.Request) {
	m, ts, err := s.publicMetrics(r)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, map[string]any{"metrics": m, "targets": ts, "rule": map[string]string{"version": s.Engine.Rule.Version, "sha256": s.Engine.Rule.SHA256}})
}

// wardStats computes ward-level aggregates only (no official is named on the
// public page — Section 13: no public shaming).
func (s *Server) wardStats(r *http.Request) ([]wardStat, error) {
	ctx := r.Context()
	wards, err := s.Engine.Store.ListWards(ctx)
	if err != nil {
		return nil, err
	}
	var out []wardStat
	for _, wd := range wards {
		if wd.Demo && !s.Cfg.Demo() {
			continue
		}
		st := wardStat{Ward: wd}
		if a, err := s.Engine.Store.ListAlerts(ctx, store.AlertFilter{WardID: wd.ID, Status: "active", Limit: 1}); err == nil && len(a) > 0 {
			st.Level = a[0].Level
		}
		cs, _ := s.Engine.Store.ListComplaints(ctx, store.ComplaintFilter{WardID: wd.ID, Limit: 20000})
		var fix []float64
		for _, c := range cs {
			if c.Status == domain.StatusInvalid {
				continue
			}
			st.Complaints++
			if c.VerifiedFixedAt != nil {
				st.Verified++
				fix = append(fix, c.VerifiedFixedAt.Sub(c.CreatedAt).Hours()/24)
			}
		}
		st.MedianFixDays = median(fix)
		es, _ := s.Engine.Store.ListEscalations(ctx, store.EscalationFilter{WardID: wd.ID, Limit: 50000})
		var ack []float64
		for _, e := range es {
			if e.AcknowledgedAt != nil {
				ack = append(ack, e.AcknowledgedAt.Sub(e.SentAt).Hours())
			}
		}
		st.MedianAckHrs = median(ack)
		out = append(out, st)
	}
	return out, nil
}

func median(xs []float64) *float64 {
	if len(xs) == 0 {
		return nil
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	m := c[len(c)/2]
	if len(c)%2 == 0 {
		m = (c[len(c)/2-1] + c[len(c)/2]) / 2
	}
	return &m
}

// ---- Officials' evidence-pack link (/o/{token}) ----

func (s *Server) escalationFromToken(r *http.Request) (domain.Escalation, error) {
	tok := r.PathValue("token")
	if len(tok) < 20 || len(tok) > 100 {
		return domain.Escalation{}, store.ErrNotFound
	}
	return s.Engine.Store.GetEscalationByToken(r.Context(), secure.HashToken(tok))
}

func (s *Server) officialPage(w http.ResponseWriter, r *http.Request) {
	esc, err := s.escalationFromToken(r)
	if err != nil {
		http.Error(w, "This link is not valid.", http.StatusNotFound)
		return
	}
	pack, err := s.evidencePack(r, esc)
	if err != nil {
		http.Error(w, "could not load evidence", http.StatusInternalServerError)
		return
	}
	pack["Token"] = r.PathValue("token")
	pack["CSRF"] = s.publicCSRF("official:" + esc.Ref)
	pack["Title"] = "Evidence pack " + esc.Ref
	pack["Map"] = true
	s.render(w, r, "official", pack)
}

func (s *Server) officialAck(w http.ResponseWriter, r *http.Request) {
	esc, err := s.escalationFromToken(r)
	if err != nil {
		http.Error(w, "This link is not valid.", http.StatusNotFound)
		return
	}
	if !s.validPublicCSRF("official:"+esc.Ref, r.FormValue("csrf")) {
		http.Error(w, "form expired; reload the page", http.StatusForbidden)
		return
	}
	o, _ := s.Engine.Store.GetOfficial(r.Context(), esc.OfficialID)
	err = s.Engine.Acknowledge(r.Context(), esc.ID, "link", strings.TrimSpace(r.FormValue("grievance")), "official:"+o.Name)
	if err != nil && !errors.Is(err, agents.ErrClosed) {
		http.Error(w, "could not record acknowledgement", http.StatusInternalServerError)
		return
	}
	redirectOK(w, r, "/o/"+r.PathValue("token"), "Acknowledged. Thank you.")
}

const maxPhoto = 10 << 20

func (s *Server) savePhoto(r *http.Request, field string) (*int64, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	if hdr.Size > maxPhoto {
		return nil, errors.New("photo is larger than 10 MB")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPhoto+1))
	if err != nil || len(data) > maxPhoto {
		return nil, errors.New("could not read photo")
	}
	ct := http.DetectContentType(data)
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		return nil, errors.New("photo must be JPEG, PNG or WebP")
	}
	sum := sha256.Sum256(data)
	id, err := s.Engine.Store.SaveMedia(r.Context(), domain.Media{SHA256: hex.EncodeToString(sum[:]), ContentType: ct, Data: data, Kind: "photo", CreatedAt: s.Engine.Clock.Now()})
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// retestFromForm records a retest entered with a fix claim.
func (s *Server) retestFromForm(r *http.Request, wardID, actor string) (*int64, error) {
	param := strings.TrimSpace(r.FormValue("retest_parameter"))
	if param == "" {
		return nil, nil
	}
	val, err := strconv.ParseFloat(r.FormValue("retest_value"), 64)
	if err != nil {
		return nil, errors.New("retest value must be a number")
	}
	lim, err := strconv.ParseFloat(r.FormValue("retest_limit"), 64)
	if err != nil {
		return nil, errors.New("retest limit must be a number")
	}
	src := domain.TestSource(r.FormValue("retest_source"))
	if !domain.Contains(domain.TestSources, src) {
		src = domain.TestLabReport
	}
	t, err := s.Engine.AddWaterTest(r.Context(), domain.WaterTest{Source: src, WardID: wardID, SampledAt: s.Engine.Clock.Now(), Parameter: param,
		Value: &val, LimitValue: &lim, Unit: strings.TrimSpace(r.FormValue("retest_unit")), IsContaminated: val > lim, Note: "submitted with fix claim"}, actor)
	if err != nil {
		return nil, err
	}
	return &t.ID, nil
}

func (s *Server) officialFixed(w http.ResponseWriter, r *http.Request) {
	esc, err := s.escalationFromToken(r)
	if err != nil {
		http.Error(w, "This link is not valid.", http.StatusNotFound)
		return
	}
	if err := r.ParseMultipartForm(maxPhoto + (1 << 20)); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	if !s.validPublicCSRF("official:"+esc.Ref, r.FormValue("csrf")) {
		http.Error(w, "form expired; reload the page", http.StatusForbidden)
		return
	}
	cid, err := strconv.ParseInt(r.FormValue("complaint_id"), 10, 64)
	if err != nil {
		http.Error(w, "choose a complaint", http.StatusBadRequest)
		return
	}
	// The complaint must belong to this escalation's scope.
	pack, err := s.evidencePack(r, esc)
	if err != nil {
		http.Error(w, "could not load evidence", http.StatusInternalServerError)
		return
	}
	allowed := false
	for _, c := range pack["Complaints"].([]domain.Complaint) {
		allowed = allowed || c.ID == cid
	}
	if !allowed {
		http.Error(w, "that complaint is not part of this escalation", http.StatusForbidden)
		return
	}
	o, _ := s.Engine.Store.GetOfficial(r.Context(), esc.OfficialID)
	actor := "official:" + o.Name
	photo, err := s.savePhoto(r, "photo")
	if err != nil {
		redirectOK(w, r, "/o/"+r.PathValue("token"), "Not saved: "+err.Error())
		return
	}
	retest, err := s.retestFromForm(r, esc.WardID, actor)
	if err != nil {
		redirectOK(w, r, "/o/"+r.PathValue("token"), "Not saved: "+err.Error())
		return
	}
	_ = s.Engine.Acknowledge(r.Context(), esc.ID, "link", "", actor)
	_, err = s.Engine.ClaimFixed(r.Context(), agents.Claim{ComplaintID: cid, EscalationID: &esc.ID, ClaimedBy: actor, PhotoMediaID: photo, RetestID: retest,
		Note: strings.TrimSpace(r.FormValue("note"))})
	if err != nil {
		redirectOK(w, r, "/o/"+r.PathValue("token"), "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/o/"+r.PathValue("token"), "Recorded. Bhagirath will now ask the resident to confirm the fix.")
}

// evidencePack gathers everything an official needs (Section 10.2).
func (s *Server) evidencePack(r *http.Request, esc domain.Escalation) (map[string]any, error) {
	ctx := r.Context()
	st := s.Engine.Store
	wd, err := st.GetWard(ctx, esc.WardID)
	if err != nil {
		return nil, err
	}
	o, _ := st.GetOfficial(ctx, esc.OfficialID)
	out := map[string]any{"Esc": esc, "Ward": wd, "Official": o, "Deadline": esc.DeadlineAt}
	var complaints []domain.Complaint
	if esc.AlertID != nil {
		a, err := st.GetAlert(ctx, *esc.AlertID)
		if err != nil {
			return nil, err
		}
		out["Alert"] = a
		var res rule.Result
		_ = jsonUnmarshal(a.Signals, &res)
		out["Signals"] = res
		since := s.Engine.Clock.Now().Add(-time.Duration(s.Engine.Rule.LookbackDays()) * 24 * time.Hour)
		all, _ := st.RuleComplaints(ctx, wd.ID, since)
		for _, c := range all {
			if c.Status != domain.StatusInvalid && c.Status != domain.StatusVerifiedFixed {
				complaints = append(complaints, c)
			}
		}
		tests, _ := st.ListWaterTests(ctx, wd.ID, since, 50)
		out["Tests"] = tests
		ill, _ := st.ListIllness(ctx, wd.ID, s.Engine.Clock.Now().Add(-7*24*time.Hour), 100)
		out["Illness"] = ill
	} else if esc.ComplaintID != nil {
		c, err := st.GetComplaint(ctx, *esc.ComplaintID)
		if err != nil {
			return nil, err
		}
		complaints = []domain.Complaint{c}
		cls, _ := st.ClosuresFor(ctx, c.ID)
		out["Closures"] = cls
	}
	out["Complaints"] = complaints
	type pin struct {
		Lat, Lon float64
		Ref      string
		Issue    string
	}
	var pins []pin
	for _, c := range complaints {
		if c.Point != nil {
			pins = append(pins, pin{c.Point.Lat, c.Point.Lon, c.Ref, string(c.IssueType)})
		}
	}
	out["Pins"] = pins
	out["Local"] = wd.DefaultLanguage
	return out, nil
}
