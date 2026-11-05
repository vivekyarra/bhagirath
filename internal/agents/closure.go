package agents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Callback schedule (Section 11): three attempts over 48 hours, then a
// no-answer decision. Offsets are from the fix claim.
var callbackOffsets = []time.Duration{0, 20 * time.Hour, 40 * time.Hour}

// CallbackDecisionAfter is when an unanswered closure is decided "no answer".
const CallbackDecisionAfter = 48 * time.Hour

// Closure verification errors.
var (
	ErrNoEvidence     = errors.New("a repair photo or a passing retest is required to claim a fix")
	ErrRetestFailed   = errors.New("the retest submitted as evidence is itself contaminated")
	ErrNotClaimable   = errors.New("complaint is not open")
	ErrAlreadyDecided = errors.New("closure already decided")
)

// Claim is an official's (or partner's) "fixed" claim.
type Claim struct {
	ComplaintID  int64
	EscalationID *int64
	ClaimedBy    string
	PhotoMediaID *int64
	RetestID     *int64
	Note         string
}

// ClaimFixed records a fix claim. Evidence is mandatory; the complaint moves to
// claimed_fixed and the complainant callback is scheduled immediately. The
// complaint is NOT verified until the resident confirms.
func (e *Engine) ClaimFixed(ctx context.Context, cl Claim) (domain.Closure, error) {
	var out domain.Closure
	if cl.PhotoMediaID == nil && cl.RetestID == nil {
		return out, ErrNoEvidence
	}
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		c, err := tx.LockComplaint(ctx, cl.ComplaintID)
		if err != nil {
			return err
		}
		switch c.Status {
		case domain.StatusOpen, domain.StatusEscalated, domain.StatusReopened:
		default:
			return ErrNotClaimable
		}
		if cl.RetestID != nil {
			t, err := tx.GetWaterTest(ctx, *cl.RetestID)
			if err != nil {
				return err
			}
			if t.IsContaminated {
				return ErrRetestFailed
			}
		}
		et := domain.EvidencePhoto
		switch {
		case cl.PhotoMediaID != nil && cl.RetestID != nil:
			et = domain.EvidenceBoth
		case cl.RetestID != nil:
			et = domain.EvidenceRetest
		}
		now := e.now()
		out = domain.Closure{ComplaintID: c.ID, ClaimedAt: now, ClaimedBy: cl.ClaimedBy, EscalationID: cl.EscalationID, EvidenceType: et,
			EvidenceMediaID: cl.PhotoMediaID, RetestID: cl.RetestID, Note: cl.Note, NextCallbackAt: &now, CallbackResult: domain.CallbackPending}
		if err := tx.CreateClosure(ctx, &out); err != nil {
			return err
		}
		c.Status = domain.StatusClaimedFixed
		if err := tx.UpdateComplaint(ctx, &c, now); err != nil {
			return err
		}
		e.event(ctx, tx, cl.ClaimedBy, "closure.claimed", "complaint", c.Ref, c.WardID, map[string]any{"evidence": et, "closure": out.ID})
		return nil
	})
	if err != nil {
		return out, err
	}
	_, err = e.ProcessCallbacks(ctx) // first call/message goes out right away
	return out, err
}

// ProcessCallbacks places due complainant callbacks and decides unanswered
// closures (scheduled every 5 minutes).
func (e *Engine) ProcessCallbacks(ctx context.Context) (int, error) {
	n := 0
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		due, err := tx.DueCallbacks(ctx, e.now(), 100)
		if err != nil {
			return err
		}
		for _, cl := range due {
			if err := e.callback(ctx, tx, cl); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (e *Engine) callback(ctx context.Context, tx *store.Store, cl domain.Closure) error {
	now := e.now()
	c, err := tx.GetComplaint(ctx, cl.ComplaintID)
	if err != nil {
		return err
	}
	var rep domain.Reporter
	contactable := false
	if c.ReporterID != nil {
		rep, err = tx.GetReporter(ctx, *c.ReporterID)
		if err != nil {
			return err
		}
		contactable = rep.CanContact()
	}
	if !contactable {
		// No consent / anonymous: the partner must verify in person.
		cl.NextCallbackAt, cl.CallbackChannel = nil, "field_visit_required"
		if err := tx.UpdateClosure(ctx, cl); err != nil {
			return err
		}
		e.event(ctx, tx, "closure-agent", "closure.field_visit_required", "complaint", c.Ref, c.WardID, nil)
		return e.notifyPartner(ctx, tx, c.WardID, fmt.Sprintf("Complaint %s was marked fixed but the resident cannot be called (no consent). Please verify in person and record the result.", c.Ref), "complaint", c.ID)
	}
	if cl.CallbackAttempts >= len(callbackOffsets) {
		if now.Sub(cl.ClaimedAt) >= CallbackDecisionAfter {
			return e.decide(ctx, tx, cl, c, domain.CallbackNoAnswer, "callback", "closure-agent")
		}
		next := cl.ClaimedAt.Add(CallbackDecisionAfter)
		cl.NextCallbackAt = &next
		return tx.UpdateClosure(ctx, cl)
	}
	l := lang.Pick(rep.PreferredLanguage, "hi")
	body := i18n.T("callback_question", l, i18n.Vars{"ref": c.Ref})
	if _, err := e.messageToReporter(ctx, tx, rep, "callback", body, false, "closure", cl.ID, Interactive{Kind: "callback", ClosureID: cl.ID}); err != nil {
		return err
	}
	cl.CallbackAttempts++
	cl.CallbackChannel = rep.PreferredChannel
	var next time.Time
	if cl.CallbackAttempts < len(callbackOffsets) {
		next = cl.ClaimedAt.Add(callbackOffsets[cl.CallbackAttempts])
	} else {
		next = cl.ClaimedAt.Add(CallbackDecisionAfter)
	}
	if !next.After(now) {
		next = now.Add(time.Hour)
	}
	cl.NextCallbackAt = &next
	if err := tx.UpdateClosure(ctx, cl); err != nil {
		return err
	}
	if rep.PreferredChannel == "whatsapp" {
		if err := tx.SetConversation(ctx, store.Conversation{ReporterID: rep.ID, State: "await_callback", ClosureID: &cl.ID, ExpiresAt: cl.ClaimedAt.Add(CallbackDecisionAfter)}, now); err != nil {
			return err
		}
	}
	e.event(ctx, tx, "closure-agent", "closure.callback_attempt", "complaint", c.Ref, c.WardID, map[string]any{"attempt": cl.CallbackAttempts, "channel": cl.CallbackChannel})
	return nil
}

// RecordCallback stores the complainant's answer (keypress, WhatsApp button,
// spoken yes/no, or a partner's field verification).
func (e *Engine) RecordCallback(ctx context.Context, closureID int64, confirmed bool, channel, actor string) error {
	return e.Store.Tx(ctx, func(tx *store.Store) error {
		cl, err := tx.LockClosure(ctx, closureID)
		if err != nil {
			return err
		}
		if cl.DecidedAt != nil {
			return ErrAlreadyDecided
		}
		c, err := tx.LockComplaint(ctx, cl.ComplaintID)
		if err != nil {
			return err
		}
		result := domain.CallbackDenied
		if confirmed {
			result = domain.CallbackConfirmed
		}
		return e.decide(ctx, tx, cl, c, result, channel, actor)
	})
}

func (e *Engine) decide(ctx context.Context, tx *store.Store, cl domain.Closure, c domain.Complaint, result domain.CallbackResult, channel, actor string) error {
	now := e.now()
	cl.CallbackResult, cl.DecidedAt, cl.NextCallbackAt = result, &now, nil
	if result != domain.CallbackNoAnswer {
		cl.CallbackAt = &now
		cl.CallbackChannel = channel
	}
	var rep *domain.Reporter
	if c.ReporterID != nil {
		r, err := tx.GetReporter(ctx, *c.ReporterID)
		if err == nil {
			rep = &r
			_ = tx.ClearConversation(ctx, r.ID)
		}
	}
	if result == domain.CallbackConfirmed {
		cl.Verified = true
		if err := tx.UpdateClosure(ctx, cl); err != nil {
			return err
		}
		c.Status, c.VerifiedFixedAt = domain.StatusVerifiedFixed, &now
		if err := tx.UpdateComplaint(ctx, &c, now); err != nil {
			return err
		}
		e.event(ctx, tx, actor, "closure.verified", "complaint", c.Ref, c.WardID, map[string]any{"channel": channel, "days_to_fix": now.Sub(c.CreatedAt).Hours() / 24})
		if err := e.resolveComplaintEscalations(ctx, tx, c.ID, now); err != nil {
			return err
		}
		if rep != nil && channel != "field_visit" {
			l := lang.Pick(rep.PreferredLanguage, "hi")
			if _, err := e.messageToReporter(ctx, tx, *rep, "callback_ack", i18n.T("callback_yes", l, i18n.Vars{"ref": c.Ref}), true, "complaint", c.ID, nil); err != nil {
				return err
			}
		}
		return nil
	}
	// Denied or unanswered: this is the "fake closure caught" path — the
	// direct answer to complaints closed without repair in Indore.
	if err := tx.UpdateClosure(ctx, cl); err != nil {
		return err
	}
	c.Status = domain.StatusReopened
	c.ReopenCount++
	if err := tx.UpdateComplaint(ctx, &c, now); err != nil {
		return err
	}
	action := "closure.denied"
	if result == domain.CallbackNoAnswer {
		action = "closure.no_answer"
	}
	e.event(ctx, tx, actor, action, "complaint", c.Ref, c.WardID, map[string]any{"channel": channel, "reopen_count": c.ReopenCount})
	e.event(ctx, tx, "closure-agent", "complaint.reopened", "complaint", c.Ref, c.WardID, map[string]any{"cause": result})
	if rep != nil && result == domain.CallbackDenied && channel != "field_visit" {
		l := lang.Pick(rep.PreferredLanguage, "hi")
		if _, err := e.messageToReporter(ctx, tx, *rep, "callback_ack", i18n.T("callback_no", l, i18n.Vars{"ref": c.Ref}), true, "complaint", c.ID, nil); err != nil {
			return err
		}
	}
	// Close the escalations that produced the false claim and go one level up.
	escs, err := tx.ListEscalations(ctx, store.EscalationFilter{ComplaintID: c.ID})
	if err != nil {
		return err
	}
	top := domain.Ladder1
	var parent *int64
	for _, x := range escs {
		if x.Level.Rank() >= top.Rank() {
			top, parent = x.Level, ptr(x.ID)
		}
		if x.Outcome == domain.OutcomePending || x.Outcome == domain.OutcomeAcknowledged {
			x.Outcome, x.ClosedAt = domain.OutcomeClosed, &now
			if err := tx.UpdateEscalation(ctx, x); err != nil {
				return err
			}
		}
	}
	next, ok := top.Next()
	if !ok {
		next = top // already at the top: re-send to the top rung with a new deadline
	}
	return e.EscalateComplaint(ctx, tx, &c, next, "reopened", parent)
}
