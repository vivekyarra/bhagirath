// Package notify delivers the outbox. Agents only ever enqueue messages; this
// dispatcher sends them with retries and records the provider's message id so
// delivery receipts can be matched. In demo mode, or for a channel with no
// configured provider, messages are marked "simulated" (demo) or "failed"
// (pilot) — never silently dropped.
package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
	"github.com/vivekyarra/bhagirath/internal/telephony"
	"github.com/vivekyarra/bhagirath/internal/whatsapp"
)

// MaxAttempts before a message is marked failed.
const MaxAttempts = 5

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// Dispatcher sends queued messages.
type Dispatcher struct {
	Store    *store.Store
	Vault    *secure.Vault
	Clock    clock.Clock
	Cfg      *config.Config
	WhatsApp *whatsapp.Client  // nil if not configured
	Twilio   *telephony.Twilio // nil if not configured
	Exotel   *telephony.Exotel // nil if not configured
	Mailer   Mailer            // nil if not configured
	Log      *slog.Logger
}

// Mailer sends email.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// ErrNoProvider means the channel has no configured provider.
var ErrNoProvider = errors.New("no provider configured for channel")

// Run delivers due messages until none remain or ctx ends. It returns the
// number processed.
func (d *Dispatcher) Run(ctx context.Context) (int, error) {
	total := 0
	for {
		n := 0
		err := d.Store.Tx(ctx, func(tx *store.Store) error {
			msgs, err := tx.DueMessages(ctx, d.Clock.Now(), 25)
			if err != nil {
				return err
			}
			n = len(msgs)
			for _, m := range msgs {
				d.deliver(ctx, tx, m)
			}
			return nil
		})
		total += n
		if err != nil || n < 25 || ctx.Err() != nil {
			return total, err
		}
	}
}

func (d *Dispatcher) deliver(ctx context.Context, tx *store.Store, m domain.OutboundMessage) {
	now := d.Clock.Now()
	m.Attempts++
	switch {
	case m.Channel == "dashboard" || m.Channel == "grievance":
		// Partner tasks are shown on the dashboard; nothing to transmit.
		m.Status, m.SentAt, m.ProviderID = domain.MsgSent, &now, "dashboard"
	case d.Cfg.Demo():
		m.Status, m.SentAt = domain.MsgSimulated, &now
	default:
		id, err := d.send(ctx, tx, m)
		switch {
		case err == nil:
			m.Status, m.SentAt, m.ProviderID, m.Error = domain.MsgSent, &now, id, ""
		case errors.Is(err, ErrNoProvider) || m.Attempts >= MaxAttempts:
			m.Status, m.Error = domain.MsgFailed, err.Error()
		default:
			m.Error = err.Error()
			m.NextAttemptAt = now.Add(backoff[min(m.Attempts-1, len(backoff)-1)])
		}
		if err != nil && d.Log != nil {
			d.Log.Warn("delivery failed", "message", m.ID, "channel", m.Channel, "attempt", m.Attempts, "err", err)
		}
	}
	if err := tx.UpdateMessage(ctx, m); err != nil && d.Log != nil {
		d.Log.Error("outbox update failed", "message", m.ID, "err", err)
	}
	if m.Entity == "advisory" && (m.Status == domain.MsgSent || m.Status == domain.MsgSimulated) {
		_ = tx.SetAdvisoryMessage(ctx, m.EntityID, m.ID, m.Status == domain.MsgSimulated)
	}
}

func (d *Dispatcher) address(ctx context.Context, tx *store.Store, m domain.OutboundMessage) (phone, email, language string, err error) {
	switch m.RecipientType {
	case "reporter":
		r, err := tx.GetReporter(ctx, m.RecipientID)
		if err != nil {
			return "", "", "", err
		}
		if len(r.PhoneEnc) == 0 {
			return "", "", "", errors.New("reporter has no contact number (consent withdrawn or purged)")
		}
		p, err := d.Vault.Decrypt(r.PhoneEnc)
		return p, "", r.PreferredLanguage, err
	case "official":
		o, err := tx.GetOfficial(ctx, m.RecipientID)
		if err != nil {
			return "", "", "", err
		}
		if len(o.PhoneEnc) > 0 {
			phone, err = d.Vault.Decrypt(o.PhoneEnc)
			if err != nil {
				return "", "", "", err
			}
		}
		return phone, o.Email, o.Language, nil
	}
	return "", "", "", fmt.Errorf("no address for recipient type %q", m.RecipientType)
}

func (d *Dispatcher) send(ctx context.Context, tx *store.Store, m domain.OutboundMessage) (string, error) {
	phone, email, _, err := d.address(ctx, tx, m)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var inter struct {
		Kind      string `json:"kind"`
		ClosureID int64  `json:"closure_id"`
	}
	if len(m.Interactive) > 0 {
		_ = json.Unmarshal(m.Interactive, &inter)
	}
	switch m.Channel {
	case "email":
		if d.Mailer == nil {
			return "", ErrNoProvider
		}
		body := m.Body
		if m.Link != "" {
			body += "\n\nEvidence pack and actions: " + m.Link
		}
		return "email", d.Mailer.Send(ctx, email, m.Subject, body)
	case "whatsapp":
		if d.WhatsApp == nil {
			return "", ErrNoProvider
		}
		body := m.Body
		if m.Link != "" {
			body += "\n\n" + m.Link
		}
		if inter.Kind == "callback" {
			yes := map[string]string{"en": "Yes, fixed", "hi": "हाँ, ठीक है", "mr": "हो, दुरुस्त", "te": "అవును"}[m.Language]
			no := map[string]string{"en": "No, not fixed", "hi": "नहीं, ठीक नहीं", "mr": "नाही", "te": "కాదు"}[m.Language]
			if yes == "" {
				yes, no = "Yes", "No"
			}
			id, err := d.WhatsApp.SendButtons(ctx, phone, body, []whatsapp.Button{
				{ID: fmt.Sprintf("cb:%d:yes", inter.ClosureID), Title: yes}, {ID: fmt.Sprintf("cb:%d:no", inter.ClosureID), Title: no}})
			if err != nil && d.Cfg.WhatsApp.CallbackTemplate != "" {
				// Outside the 24-hour window an approved template is required.
				return d.WhatsApp.SendTemplate(ctx, phone, d.Cfg.WhatsApp.CallbackTemplate, m.Language, nil)
			}
			return id, err
		}
		id, err := d.WhatsApp.SendText(ctx, phone, body)
		if err != nil && m.Kind == "advisory" && d.Cfg.WhatsApp.AdvisoryTemplate != "" {
			return d.WhatsApp.SendTemplate(ctx, phone, d.Cfg.WhatsApp.AdvisoryTemplate, m.Language, nil)
		}
		return id, err
	case "voice":
		return d.call(ctx, phone, m, inter.Kind, inter.ClosureID)
	}
	return "", fmt.Errorf("%w: %s", ErrNoProvider, m.Channel)
}

// call delivers a voice message: a callback question with a keypad answer, or
// a spoken message (advisory) in the recipient's language.
func (d *Dispatcher) call(ctx context.Context, phone string, m domain.OutboundMessage, kind string, closureID int64) (string, error) {
	switch {
	case d.Twilio != nil:
		voice := i18n.TwilioVoice[m.Language]
		if voice[0] == "" {
			voice = i18n.TwilioVoice["hi"]
		}
		say := func(text string) telephony.Say { return telephony.Say{Voice: voice[0], Language: voice[1], Text: text} }
		doc := &telephony.TwiML{}
		if kind == "callback" {
			action := d.Cfg.BaseURL + "/voice/twilio/callback?closure=" + strconv.FormatInt(closureID, 10)
			g := telephony.Gather{Action: action, Method: "POST", NumDigits: 1, Timeout: 10, Says: []telephony.Say{say(i18n.T("ivr_callback", m.Language, nil))}}
			doc.Add(g, g, say(i18n.T("ivr_thanks", m.Language, nil)))
		} else {
			doc.Add(say(i18n.T("ivr_advisory_intro", m.Language, nil)), say(m.Body), telephony.Pause{Length: 1}, say(m.Body))
		}
		return d.Twilio.Call(ctx, phone, doc.String(), "", d.Cfg.BaseURL+"/voice/twilio/status")
	case d.Exotel != nil:
		custom := "msg:" + strconv.FormatInt(m.ID, 10)
		if kind == "callback" {
			custom = "closure:" + strconv.FormatInt(closureID, 10)
		}
		return d.Exotel.Connect(ctx, phone, custom, d.Cfg.BaseURL+"/voice/exotel/status?key="+d.Cfg.Exotel.WebhookKey)
	}
	return "", ErrNoProvider
}

// SMTPMailer sends UTF-8 plain-text email with STARTTLS.
type SMTPMailer struct {
	Host, User, Password, From string
	Port                       int
}

// Send implements Mailer.
func (s *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	if to == "" {
		return errors.New("official has no email address")
	}
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return errors.New("invalid header value")
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	msg := strings.Join([]string{
		"From: " + s.From, "To: " + to, "Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: 8bit",
		"Date: " + time.Now().UTC().Format(time.RFC1123Z), "", body,
	}, "\r\n")
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Password, s.Host)
	}
	if s.Port == 465 {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		c, err := smtp.NewClient(conn, s.Host)
		if err != nil {
			return err
		}
		defer c.Close()
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		if err := c.Mail(s.From); err != nil {
			return err
		}
		if err := c.Rcpt(to); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return c.Quit()
	}
	return smtp.SendMail(addr, auth, s.From, []string{to}, []byte(msg))
}
