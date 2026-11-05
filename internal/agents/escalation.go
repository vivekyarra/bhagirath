package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// RungsFor returns the ladder rungs an alert level escalates to (Section 8.2).
func RungsFor(l domain.Level) []domain.LadderLevel {
	switch l {
	case domain.LevelCritical:
		return []domain.LadderLevel{domain.Ladder1, domain.Ladder2, domain.Ladder3, domain.LadderHealth, domain.LadderPartner}
	case domain.LevelWarning:
		return []domain.LadderLevel{domain.Ladder1, domain.Ladder2, domain.LadderHealth}
	case domain.LevelWatch:
		return []domain.LadderLevel{domain.Ladder1}
	}
	return nil
}

// EscalateAlert sends evidence packs for an alert to every official on the
// rungs its level requires who does not already hold an open escalation.
func (e *Engine) EscalateAlert(ctx context.Context, tx *store.Store, a domain.Alert, level domain.Level, reason string) error {
	existing, err := tx.ListEscalations(ctx, store.EscalationFilter{AlertID: a.ID, Outcomes: []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}})
	if err != nil {
		return err
	}
	has := map[int64]bool{}
	for _, x := range existing {
		has[x.OfficialID] = true
	}
	deadline := e.now().Add(time.Duration(e.Ladder.Deadline(level)) * time.Hour)
	for _, rung := range RungsFor(level) {
		offs, err := tx.OfficialsAt(ctx, a.WardID, rung)
		if err != nil {
			return err
		}
		if len(offs) == 0 {
			e.event(ctx, tx, "escalation-agent", "ladder.gap", "alert", a.ID, a.WardID, map[string]any{"rung": rung, "level": level})
			continue
		}
		for _, o := range offs {
			if has[o.ID] {
				continue
			}
			if _, err := e.createEscalation(ctx, tx, o, a.WardID, &a.ID, nil, rung, deadline, reason, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// EscalateComplaint sends one complaint to the officials at a rung.
func (e *Engine) EscalateComplaint(ctx context.Context, tx *store.Store, c *domain.Complaint, rung domain.LadderLevel, reason string, parent *int64) error {
	offs, err := tx.OfficialsAt(ctx, c.WardID, rung)
	if err != nil {
		return err
	}
	if len(offs) == 0 {
		e.event(ctx, tx, "escalation-agent", "ladder.gap", "complaint", c.Ref, c.WardID, map[string]any{"rung": rung})
		return nil
	}
	deadline := e.now().Add(time.Duration(e.Ladder.ComplaintDeadline()) * time.Hour)
	for _, o := range offs {
		if _, err := e.createEscalation(ctx, tx, o, c.WardID, nil, &c.ID, rung, deadline, reason, parent); err != nil {
			return err
		}
	}
	if c.Status == domain.StatusOpen || c.Status == domain.StatusReopened {
		c.Status = domain.StatusEscalated
		if err := tx.UpdateComplaint(ctx, c, e.now()); err != nil {
			return err
		}
	}
	return nil
}

// OfficialLink is the URL an official uses to acknowledge and close.
func (e *Engine) OfficialLink(token string) string { return e.Cfg.BaseURL + "/o/" + token }

func (e *Engine) createEscalation(ctx context.Context, tx *store.Store, o domain.Official, wardID string, alertID, complaintID *int64,
	rung domain.LadderLevel, deadline time.Time, reason string, parent *int64) (domain.Escalation, error) {
	token := secure.NewToken()
	channel := o.PreferredChannel
	if channel == "" {
		channel = "email"
	}
	if channel == "email" && o.Email == "" {
		channel = "whatsapp"
	}
	if channel == "whatsapp" && len(o.PhoneEnc) == 0 {
		channel = "grievance"
	}
	esc := domain.Escalation{AlertID: alertID, ComplaintID: complaintID, OfficialID: o.ID, Level: rung, WardID: wardID, SentAt: e.now(),
		Channel: channel, TokenHash: secure.HashToken(token), DeadlineAt: deadline, Outcome: domain.OutcomePending, ParentID: parent, Reason: reason}
	if err := tx.CreateEscalation(ctx, &esc); err != nil {
		return esc, err
	}
	subject, body, err := e.evidenceSummary(ctx, tx, esc, o)
	if err != nil {
		return esc, err
	}
	link := e.OfficialLink(token)
	m := domain.OutboundMessage{Kind: "escalation", Channel: channel, RecipientType: "official", RecipientID: o.ID, RecipientLabel: o.Name + " (" + o.Role + ")",
		Language: "en", Subject: subject, Body: body, Link: link, Entity: "escalation", EntityID: esc.ID, Status: domain.MsgQueued,
		CreatedAt: e.now(), NextAttemptAt: e.now()}
	if channel == "whatsapp" {
		m.Body += "\n\nReply \"ACK " + esc.Ref + "\" (or tap Acknowledge) to confirm you have received this."
		m.Interactive, _ = json.Marshal(Interactive{Kind: "ack", EscalationID: esc.ID})
	}
	if channel == "grievance" {
		// No direct contact: the partner files it on the official grievance
		// channel and records the grievance number, which Bhagirath tracks.
		m.RecipientType, m.RecipientLabel = "partner", "Partner — file on "+defaultTo(o.GrievanceChannel, "the official grievance portal")+" for "+o.Name
		m.Body = "ACTION FOR PARTNER: file this on " + defaultTo(o.GrievanceChannel, "the official grievance channel") + " and record the grievance number on the escalation page.\n\n" + body
	}
	if err := tx.EnqueueMessage(ctx, &m); err != nil {
		return esc, err
	}
	e.event(ctx, tx, "escalation-agent", "escalation.sent", "escalation", esc.Ref, wardID, map[string]any{
		"official": o.Name, "rung": rung, "channel": channel, "deadline": deadline, "reason": reason, "alert": alertID, "complaint": complaintID})
	return esc, nil
}

func defaultTo(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// evidenceSummary writes the text part of the evidence pack (the full pack,
// with map and history, is the official's link). Factual and polite: no
// shaming, no personal data about residents (Section 13).
func (e *Engine) evidenceSummary(ctx context.Context, tx *store.Store, esc domain.Escalation, o domain.Official) (string, string, error) {
	w, err := tx.GetWard(ctx, esc.WardID)
	if err != nil {
		return "", "", err
	}
	dl := esc.DeadlineAt.In(clock.IST).Format("2 Jan 2006, 3:04 PM IST")
	var b strings.Builder
	var subject string
	fmt.Fprintf(&b, "Dear %s,\n\n", o.Role)
	switch {
	case esc.AlertID != nil:
		a, err := tx.GetAlert(ctx, *esc.AlertID)
		if err != nil {
			return "", "", err
		}
		subject = fmt.Sprintf("[Bhagirath] %s water-safety alert — %s — response needed by %s", a.Level.Title(), w.Name, dl)
		fmt.Fprintf(&b, "Bhagirath, a citizen water-safety tool run with %s, has raised a %s alert for %s.\n\n", e.partnerName(w), strings.ToUpper(a.Level.String()), w.Name)
		fmt.Fprintf(&b, "Reason: %s\n", a.ReasonText)
		if a.ReasonLocal != "" {
			fmt.Fprintf(&b, "कारण: %s\n", a.ReasonLocal)
		}
	case esc.ComplaintID != nil:
		c, err := tx.GetComplaint(ctx, *esc.ComplaintID)
		if err != nil {
			return "", "", err
		}
		subject = fmt.Sprintf("[Bhagirath] Resident complaint %s — %s — %s — response needed by %s", c.Ref, extract.IssueLabel(c.IssueType), w.Name, dl)
		fmt.Fprintf(&b, "A resident of %s reported: %s.\n", w.Name, c.DescriptionEN)
		if c.LocationText != "" {
			fmt.Fprintf(&b, "Location: %s\n", c.LocationText)
		}
		fmt.Fprintf(&b, "Reported: %s via %s.\n", c.CreatedAt.In(clock.IST).Format("2 Jan 2006, 3:04 PM"), strings.ReplaceAll(string(c.Channel), "_", " "))
		if esc.Reason == "reopened" {
			b.WriteString("\nThis complaint was previously marked fixed, but the resident reports the problem is NOT fixed. It has been reopened and escalated to you.\n")
		}
	}
	if esc.Reason == "timeout" {
		b.WriteString("\nThis has been escalated to you because the previous officer did not respond before the deadline.\n")
	}
	if esc.Reason == "ack_no_fix" {
		b.WriteString("\nThis has been escalated to you because the issue was acknowledged but not fixed within the deadline.\n")
	}
	fmt.Fprintf(&b, "\nPlease acknowledge by %s. The evidence pack (map, anonymised complaints, test results, illness counts) and a form to mark the problem fixed with a photo or a passing retest are at the link below. A fix is counted only after the resident confirms it.\n", dl)
	fmt.Fprintf(&b, "\nReference: %s\n\nBhagirath is not a government body; this is a factual notification on behalf of residents.", esc.Ref)
	return subject, b.String(), nil
}

// ErrClosed is returned for actions on finished escalations.
var ErrClosed = errors.New("escalation is closed")

// Acknowledge records an official's acknowledgement (via link, WhatsApp
// reply, or the partner recording a grievance number). For a complaint, the
// acknowledgement starts a fix window; if no verified fix claim arrives in
// time, the next rung is notified ("acknowledged but not fixed").
func (e *Engine) Acknowledge(ctx context.Context, escID int64, via, grievanceNumber, actor string) error {
	return e.Store.Tx(ctx, func(tx *store.Store) error {
		esc, err := tx.LockEscalation(ctx, escID)
		if err != nil {
			return err
		}
		if esc.Outcome != domain.OutcomePending {
			if grievanceNumber != "" && esc.GrievanceNumber == "" {
				esc.GrievanceNumber = grievanceNumber
				return tx.UpdateEscalation(ctx, esc)
			}
			return ErrClosed
		}
		now := e.now()
		esc.Outcome, esc.AcknowledgedAt, esc.AckVia = domain.OutcomeAcknowledged, &now, via
		if grievanceNumber != "" {
			esc.GrievanceNumber = strings.TrimSpace(grievanceNumber)
		}
		if esc.ComplaintID != nil {
			esc.DeadlineAt = now.Add(time.Duration(e.Ladder.ComplaintDeadline()) * time.Hour)
		}
		if err := tx.UpdateEscalation(ctx, esc); err != nil {
			return err
		}
		e.event(ctx, tx, actor, "escalation.acknowledged", "escalation", esc.Ref, esc.WardID, map[string]any{"via": via, "grievance_number": esc.GrievanceNumber,
			"hours_to_ack": now.Sub(esc.SentAt).Hours()})
		return nil
	})
}

// ProcessTimeouts escalates every escalation whose timer expired to the next
// rung (scheduled every 5 minutes). It returns how many timed out.
func (e *Engine) ProcessTimeouts(ctx context.Context) (int, error) {
	n := 0
	for {
		batch := 0
		err := e.Store.Tx(ctx, func(tx *store.Store) error {
			due, err := tx.DueEscalations(ctx, e.now(), 50)
			if err != nil {
				return err
			}
			batch = len(due)
			for _, esc := range due {
				if err := e.timeout(ctx, tx, esc); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return n, err
		}
		n += batch
		if batch < 50 {
			return n, nil
		}
	}
}

func (e *Engine) timeout(ctx context.Context, tx *store.Store, esc domain.Escalation) error {
	now := e.now()
	reason := "timeout"
	if esc.Outcome == domain.OutcomeAcknowledged {
		reason = "ack_no_fix"
		if esc.ComplaintID != nil {
			c, err := tx.GetComplaint(ctx, *esc.ComplaintID)
			if err != nil {
				return err
			}
			if c.Status == domain.StatusClaimedFixed || c.Status == domain.StatusVerifiedFixed || c.Status == domain.StatusInvalid {
				esc.Outcome, esc.ClosedAt = domain.OutcomeResolved, &now
				return tx.UpdateEscalation(ctx, esc)
			}
		}
	}
	esc.Outcome, esc.ClosedAt = domain.OutcomeTimedOut, &now
	if err := tx.UpdateEscalation(ctx, esc); err != nil {
		return err
	}
	e.event(ctx, tx, "escalation-agent", "escalation.timeout", "escalation", esc.Ref, esc.WardID, map[string]any{"rung": esc.Level, "reason": reason})
	next, ok := esc.Level.Next()
	if !ok {
		e.event(ctx, tx, "escalation-agent", "ladder.exhausted", "escalation", esc.Ref, esc.WardID, map[string]any{"rung": esc.Level})
		return e.notifyPartner(ctx, tx, esc.WardID, fmt.Sprintf("Escalation %s reached the top of the ladder without a response. Consider the official grievance portal, the councillor, or media.", esc.Ref), "escalation", esc.ID)
	}
	offs, err := tx.OfficialsAt(ctx, esc.WardID, next)
	if err != nil {
		return err
	}
	if len(offs) == 0 {
		e.event(ctx, tx, "escalation-agent", "ladder.gap", "escalation", esc.Ref, esc.WardID, map[string]any{"rung": next})
		return nil
	}
	deadline := now.Add(esc.DeadlineAt.Sub(esc.SentAt))
	if esc.Outcome == domain.OutcomeTimedOut && reason == "ack_no_fix" {
		deadline = now.Add(time.Duration(e.Ladder.ComplaintDeadline()) * time.Hour)
	}
	if deadline.Sub(now) < time.Hour {
		deadline = now.Add(24 * time.Hour)
	}
	if esc.AlertID != nil {
		a, err := tx.GetAlert(ctx, *esc.AlertID)
		if err != nil {
			return err
		}
		if a.Status != "active" {
			return nil
		}
		existing, err := tx.ListEscalations(ctx, store.EscalationFilter{AlertID: a.ID, Outcomes: []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}})
		if err != nil {
			return err
		}
		for _, o := range offs {
			dup := false
			for _, x := range existing {
				dup = dup || x.OfficialID == o.ID
			}
			if !dup {
				if _, err := e.createEscalation(ctx, tx, o, esc.WardID, esc.AlertID, nil, next, deadline, reason, &esc.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	c, err := tx.GetComplaint(ctx, *esc.ComplaintID)
	if err != nil {
		return err
	}
	if c.Status.Closed() {
		return nil
	}
	for _, o := range offs {
		if _, err := e.createEscalation(ctx, tx, o, esc.WardID, nil, esc.ComplaintID, next, deadline, reason, &esc.ID); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) notifyPartner(ctx context.Context, tx *store.Store, wardID, body, entity string, entityID int64) error {
	m := domain.OutboundMessage{Kind: "partner", Channel: "dashboard", RecipientType: "partner", RecipientLabel: "Partner (" + wardID + ")",
		Language: "en", Body: body, Entity: entity, EntityID: entityID, Status: domain.MsgQueued, CreatedAt: e.now(), NextAttemptAt: e.now()}
	return tx.EnqueueMessage(ctx, &m)
}

func (e *Engine) closeAlertEscalations(ctx context.Context, tx *store.Store, alertID int64, now time.Time) error {
	open, err := tx.ListEscalations(ctx, store.EscalationFilter{AlertID: alertID, Outcomes: []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}})
	if err != nil {
		return err
	}
	for _, x := range open {
		x.Outcome, x.ClosedAt = domain.OutcomeClosed, &now
		if err := tx.UpdateEscalation(ctx, x); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) resolveComplaintEscalations(ctx context.Context, tx *store.Store, complaintID int64, now time.Time) error {
	open, err := tx.ListEscalations(ctx, store.EscalationFilter{ComplaintID: complaintID, Outcomes: []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}})
	if err != nil {
		return err
	}
	for _, x := range open {
		x.Outcome, x.ClosedAt = domain.OutcomeResolved, &now
		if err := tx.UpdateEscalation(ctx, x); err != nil {
			return err
		}
	}
	return nil
}
