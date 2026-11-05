package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/locate"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Message is one inbound WhatsApp (or similar chat) message.
type Message struct {
	ProviderMsgID string
	Phone         string
	Text          string
	ReplyID       string // interactive button payload, e.g. "cb:42:yes"
	Point         *domain.Point
	LocationText  string // name/address attached to a shared location
	Audio         []byte
	AudioType     string
	Actor         string
}

// Outcome describes how a message was handled (for logs and the simulator).
type Outcome struct {
	Handled   string // complaint | consent | location | callback | illness | ignored
	Complaint *domain.Complaint
	Detail    string
}

var yesWords = []string{"yes", "y", "1", "haan", "han", "ha", "हाँ", "हां", "हा", "हो", "ho", "avunu", "అవును", "ok", "fixed", "theek"}
var noWords = []string{"no", "n", "2", "nahi", "nahin", "नहीं", "नही", "नाही", "kaadu", "కాదు", "లేదు", "not fixed"}

func answer(text string) (yes, no bool) {
	t := lang.Normalize(text)
	for _, w := range yesWords {
		if t == lang.Normalize(w) {
			return true, false
		}
	}
	for _, w := range noWords {
		if t == lang.Normalize(w) {
			return false, true
		}
	}
	return false, false
}

// HandleMessage routes a chat message: callback answers, consent replies,
// location follow-ups, clinic illness counts, or a new complaint.
func (e *Engine) HandleMessage(ctx context.Context, m Message) (Outcome, error) {
	if m.Actor == "" {
		m.Actor = "resident"
	}
	phone, err := secure.NormalizePhone(m.Phone)
	if err != nil {
		return Outcome{Handled: "ignored", Detail: "invalid sender"}, nil
	}
	rep, err := e.Store.GetReporterByHash(ctx, e.Vault.HashPhone(phone))
	known := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Outcome{}, err
	}

	// Button replies carry their own target and work even after state expiry.
	if strings.HasPrefix(m.ReplyID, "cb:") {
		parts := strings.Split(m.ReplyID, ":")
		if len(parts) == 3 {
			var id int64
			if _, err := fmt.Sscan(parts[1], &id); err == nil && known && e.closureBelongsTo(ctx, id, rep.ID) {
				err := e.RecordCallback(ctx, id, parts[2] == "yes", "whatsapp", m.Actor)
				if errors.Is(err, ErrAlreadyDecided) {
					return Outcome{Handled: "ignored", Detail: "already decided"}, nil
				}
				return Outcome{Handled: "callback", Detail: parts[2]}, err
			}
		}
		return Outcome{Handled: "ignored", Detail: "unknown button"}, nil
	}

	if known && rep.Kind != domain.KindResident && rep.Kind.IllnessSource() {
		return e.clinicMessage(ctx, rep, m)
	}

	if known {
		conv, err := e.Store.GetConversation(ctx, rep.ID, e.now())
		if err == nil {
			switch conv.State {
			case "await_callback":
				if y, n := answer(m.Text); (y || n) && conv.ClosureID != nil {
					err := e.RecordCallback(ctx, *conv.ClosureID, y, "whatsapp", m.Actor)
					if errors.Is(err, ErrAlreadyDecided) {
						err = nil
					}
					return Outcome{Handled: "callback", Detail: map[bool]string{true: "yes", false: "no"}[y]}, err
				}
			case "await_consent":
				if y, n := answer(m.Text); y || n {
					return e.consentReply(ctx, rep, y, conv)
				}
			case "await_location":
				if conv.ComplaintID != nil && (m.Point != nil || (strings.TrimSpace(m.Text) != "" && len(m.Audio) == 0)) {
					out, done, err := e.locationReply(ctx, rep, *conv.ComplaintID, m)
					if err != nil || done {
						return out, err
					}
				}
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return Outcome{}, err
		}
	}
	pinKey := "pin:" + e.Vault.HashPhone(phone)
	if m.Point != nil && strings.TrimSpace(m.Text) == "" && len(m.Audio) == 0 {
		// A pin on its own: attach it to the sender's recent complaint that has
		// no point, or keep it for the complaint they are about to send.
		if known {
			recent, err := e.Store.RecentByReporter(ctx, rep.ID, e.now().Add(-24*time.Hour))
			if err != nil {
				return Outcome{}, err
			}
			for _, c := range recent {
				if c.Point == nil && !c.Status.Closed() {
					out, done, err := e.locationReply(ctx, rep, c.ID, Message{Point: m.Point, Text: m.LocationText})
					if err != nil || done {
						return out, err
					}
				}
			}
		}
		val := fmt.Sprintf("%f,%f,%d,%s", m.Point.Lat, m.Point.Lon, e.now().Unix(), strings.ReplaceAll(m.LocationText, ",", " "))
		if err := e.Store.KVSet(ctx, pinKey, val, e.now()); err != nil {
			return Outcome{}, err
		}
		return Outcome{Handled: "location", Detail: "pin kept for the next complaint"}, nil
	}
	if strings.TrimSpace(m.Text) == "" && len(m.Audio) == 0 {
		return Outcome{Handled: "ignored", Detail: "empty"}, nil
	}
	if m.Point == nil {
		if v, err := e.Store.KVGet(ctx, pinKey); err == nil {
			var lat, lon float64
			var at int64
			var name string
			parts := strings.SplitN(v, ",", 4)
			if len(parts) >= 3 {
				_, e1 := fmt.Sscan(parts[0], &lat)
				_, e2 := fmt.Sscan(parts[1], &lon)
				_, e3 := fmt.Sscan(parts[2], &at)
				if len(parts) == 4 {
					name = parts[3]
				}
				if e1 == nil && e2 == nil && e3 == nil && e.now().Unix()-at < 2*3600 {
					m.Point, m.LocationText = &domain.Point{Lat: lat, Lon: lon}, name
				}
			}
		}
	}
	res, err := e.Intake(ctx, Inbound{Channel: domain.ChannelWhatsApp, ProviderMsgID: m.ProviderMsgID, Phone: m.Phone, Text: m.Text,
		Audio: m.Audio, AudioType: m.AudioType, Point: m.Point, LocationText: m.LocationText, Actor: m.Actor})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Handled: "complaint", Complaint: &res.Complaint}, nil
}

func (e *Engine) closureBelongsTo(ctx context.Context, closureID, reporterID int64) bool {
	cl, err := e.Store.GetClosure(ctx, closureID)
	if err != nil {
		return false
	}
	c, err := e.Store.GetComplaint(ctx, cl.ComplaintID)
	return err == nil && c.ReporterID != nil && *c.ReporterID == reporterID
}

func (e *Engine) consentReply(ctx context.Context, rep domain.Reporter, yes bool, conv store.Conversation) (Outcome, error) {
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		now := e.now()
		l := lang.Pick(rep.PreferredLanguage, "hi")
		if yes {
			rep.ConsentAt, rep.ConsentDeclined = &now, false
		} else {
			rep.ConsentDeclined, rep.PhoneEnc = true, nil
		}
		// Send the reply before dropping the number on a refusal.
		key := map[bool]string{true: "consent_thanks", false: "consent_declined"}[yes]
		if !yes {
			enc := rep
			r2, _ := tx.GetReporter(ctx, rep.ID)
			enc.PhoneEnc = r2.PhoneEnc
			if _, err := e.messageToReporter(ctx, tx, enc, "consent", i18n.T(key, l, nil), true, "reporter", rep.ID, nil); err != nil {
				return err
			}
		}
		if err := tx.UpdateReporter(ctx, rep); err != nil {
			return err
		}
		if yes {
			if _, err := e.messageToReporter(ctx, tx, rep, "consent", i18n.T(key, l, nil), true, "reporter", rep.ID, nil); err != nil {
				return err
			}
		}
		e.event(ctx, tx, rep.Ref, map[bool]string{true: "consent.granted", false: "consent.declined"}[yes], "reporter", rep.Ref, rep.WardID, nil)
		if err := tx.ClearConversation(ctx, rep.ID); err != nil {
			return err
		}
		// Continue with the location follow-up if the complaint still needs it.
		if conv.ComplaintID != nil && yes {
			c, err := tx.GetComplaint(ctx, *conv.ComplaintID)
			if err == nil && c.NeedsFollowup {
				return e.askLocation(ctx, tx, rep, c)
			}
		}
		return nil
	})
	return Outcome{Handled: "consent", Detail: fmt.Sprint(yes)}, err
}

// locationReply attaches a pin/landmark to a complaint awaiting location.
// It returns done=false when the text does not resolve (so it is treated as
// a new complaint instead).
func (e *Engine) locationReply(ctx context.Context, rep domain.Reporter, complaintID int64, m Message) (Outcome, bool, error) {
	wards, err := e.Store.ListWards(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	loc := locate.Resolve(locate.Input{Point: m.Point, Text: m.Text, ReporterWard: rep.WardID}, wards)
	if !loc.Resolved() {
		return Outcome{}, false, nil
	}
	var updated domain.Complaint
	newlyLocated := false
	err = e.Store.Tx(ctx, func(tx *store.Store) error {
		c, err := tx.LockComplaint(ctx, complaintID)
		if err != nil {
			return err
		}
		newlyLocated = c.WardID == ""
		c.WardID, c.Point, c.LocationMethod, c.NeedsFollowup = loc.WardID, loc.Point, loc.Method, loc.Point == nil
		if loc.Landmark != "" {
			c.LocationText = strings.TrimSpace(c.LocationText + " (landmark: " + loc.Landmark + ")")
		} else if m.Text != "" {
			c.LocationText = strings.TrimSpace(c.LocationText + " " + m.Text)
		}
		if err := tx.UpdateComplaint(ctx, &c, e.now()); err != nil {
			return err
		}
		updated = c
		e.event(ctx, tx, rep.Ref, "complaint.located", "complaint", c.Ref, c.WardID, map[string]any{"method": loc.Method})
		if err := tx.ClearConversation(ctx, rep.ID); err != nil {
			return err
		}
		w, _ := tx.GetWard(ctx, c.WardID)
		l := lang.Pick(rep.PreferredLanguage, "hi")
		if _, err := e.messageToReporter(ctx, tx, rep, "followup", i18n.T("location_thanks", l, i18n.Vars{"ref": c.Ref, "ward": w.Name}), true, "complaint", c.ID, nil); err != nil {
			return err
		}
		if newlyLocated && c.DuplicateOf == nil && c.IssueType != domain.IssueOther {
			return e.EscalateComplaint(ctx, tx, &updated, domain.Ladder1, "complaint", nil)
		}
		return nil
	})
	if err != nil {
		return Outcome{}, true, err
	}
	if _, err := e.EvaluateWard(ctx, updated.WardID); err != nil && e.Log != nil {
		e.Log.Error("evaluate after location", "err", err)
	}
	return Outcome{Handled: "location", Complaint: &updated}, true, nil
}

func (e *Engine) clinicMessage(ctx context.Context, rep domain.Reporter, m Message) (Outcome, error) {
	l := lang.Pick(rep.PreferredLanguage, "hi")
	counts, ok := ParseIllness(m.Text)
	if !ok || rep.WardID == "" {
		err := e.Store.Tx(ctx, func(tx *store.Store) error {
			_, err := e.messageToReporter(ctx, tx, rep, "illness_help", i18n.T("illness_format_help", l, nil), true, "reporter", rep.ID, nil)
			return err
		})
		return Outcome{Handled: "ignored", Detail: "unparsed illness message"}, err
	}
	if fresh, err := e.Store.MarkInboundSeen(ctx, "whatsapp", m.ProviderMsgID, e.now()); err != nil || !fresh {
		return Outcome{Handled: "ignored", Detail: "duplicate delivery"}, err
	}
	ir, err := e.AddIllness(ctx, domain.IllnessReport{ReporterType: rep.Kind, ReporterID: &rep.ID, WardID: rep.WardID, ReportedAt: e.now(),
		CasesDiarrhoea: counts.Diarrhoea, CasesVomiting: counts.Vomiting, ORSUnitsSold: counts.ORS, Notes: "via WhatsApp"}, rep.Ref)
	if err != nil {
		return Outcome{}, err
	}
	w, _ := e.Store.GetWard(ctx, rep.WardID)
	summary := fmt.Sprintf("D %d, V %d", counts.Diarrhoea, counts.Vomiting)
	if counts.ORS != nil {
		summary += fmt.Sprintf(", ORS %d", *counts.ORS)
	}
	err = e.Store.Tx(ctx, func(tx *store.Store) error {
		_, err := e.messageToReporter(ctx, tx, rep, "illness_ack", i18n.T("illness_ack", l, i18n.Vars{"ref": ir.Ref, "ward": w.Name, "summary": summary}), true, "illness_report", ir.ID, nil)
		return err
	})
	return Outcome{Handled: "illness", Detail: ir.Ref}, err
}

// RegisterSignalReporter enrols a clinic, chemist, ASHA or hospital (by
// phone) so their WhatsApp messages are parsed as daily illness counts.
func (e *Engine) RegisterSignalReporter(ctx context.Context, phone string, kind domain.ReporterKind, label, wardID, language, actor string) (domain.Reporter, error) {
	if kind == domain.KindResident || !kind.IllnessSource() {
		return domain.Reporter{}, errors.New("kind must be clinic, chemist, asha or hospital")
	}
	p, err := secure.NormalizePhone(phone)
	if err != nil {
		return domain.Reporter{}, err
	}
	hash := e.Vault.HashPhone(p)
	enc, err := e.Vault.Encrypt(p)
	if err != nil {
		return domain.Reporter{}, err
	}
	now := e.now()
	var out domain.Reporter
	err = e.Store.Tx(ctx, func(tx *store.Store) error {
		r, _, err := tx.CreateReporter(ctx, domain.Reporter{Ref: secure.PublicRef("S", hash), PhoneHash: hash, PhoneEnc: enc, Kind: kind, Label: label,
			PreferredLanguage: lang.Pick(language, "hi"), PreferredChannel: "whatsapp", WardID: wardID, ConsentAt: &now, CreatedAt: now})
		if err != nil {
			return err
		}
		r.Kind, r.Label, r.WardID, r.PhoneEnc, r.ConsentAt, r.PreferredChannel = kind, label, wardID, enc, &now, "whatsapp"
		if err := tx.UpdateReporter(ctx, r); err != nil {
			return err
		}
		out = r
		e.event(ctx, tx, actor, "signal_reporter.registered", "reporter", r.Ref, wardID, map[string]any{"kind": kind})
		return nil
	})
	return out, err
}
