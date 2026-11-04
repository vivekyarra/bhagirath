package agents

import (
	"context"
	"errors"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// ErrAdvisoryNotReviewed blocks real advisories until a doctor/public-health
// professional has reviewed the template text (spec Section 12).
var ErrAdvisoryNotReviewed = errors.New("advisory text has not been reviewed by a health professional (set BHAGIRATH_ADVISORY_REVIEWED_BY)")

// OnAlertWarning is called when an alert first reaches Warning or Critical.
// With partner sign-off required (the pilot default) it waits for approval.
func (e *Engine) OnAlertWarning(ctx context.Context, alertID int64) error {
	a, err := e.Store.GetAlert(ctx, alertID)
	if err != nil {
		return err
	}
	if a.Level < domain.LevelWarning || a.Status != "active" {
		return nil
	}
	if e.Cfg.RequirePartnerSignoff && a.AdvisoryApprovedAt == nil {
		return e.Store.Tx(ctx, func(tx *store.Store) error {
			e.event(ctx, tx, "advisory-agent", "advisory.awaiting_signoff", "alert", a.ID, a.WardID, map[string]any{"level": a.Level})
			return e.notifyPartner(ctx, tx, a.WardID, "Alert reached "+a.Level.Title()+". Review and approve precautionary household messages on the alert page: "+a.ReasonText, "alert", a.ID)
		})
	}
	_, err = e.SendAdvisories(ctx, alertID)
	return err
}

// ApproveAdvisories records partner sign-off and sends the messages.
func (e *Engine) ApproveAdvisories(ctx context.Context, alertID int64, by string) (int, error) {
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		a, err := tx.GetAlert(ctx, alertID)
		if err != nil {
			return err
		}
		if a.AdvisoryApprovedAt == nil {
			now := e.now()
			a.AdvisoryApprovedAt, a.AdvisoryApprovedBy = &now, by
			if err := tx.UpdateAlert(ctx, &a); err != nil {
				return err
			}
			e.event(ctx, tx, by, "advisory.approved", "alert", a.ID, a.WardID, nil)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return e.SendAdvisories(ctx, alertID)
}

// SendAdvisories messages every consenting registered household in the ward,
// in its own language, once per alert level. Idempotent.
func (e *Engine) SendAdvisories(ctx context.Context, alertID int64) (int, error) {
	if e.Cfg.AdvisoryReviewedBy == "" && !e.Cfg.Demo() {
		_ = e.Store.Tx(ctx, func(tx *store.Store) error {
			e.event(ctx, tx, "advisory-agent", "advisory.blocked_unreviewed", "alert", alertID, "", nil)
			return nil
		})
		return 0, ErrAdvisoryNotReviewed
	}
	sent := 0
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		a, err := tx.GetAlert(ctx, alertID)
		if err != nil {
			return err
		}
		if a.Status != "active" || a.Level < domain.LevelWarning {
			return nil
		}
		w, err := tx.GetWard(ctx, a.WardID)
		if err != nil {
			return err
		}
		reps, err := tx.ListReporters(ctx, store.ReporterFilter{WardID: a.WardID, Kinds: []domain.ReporterKind{domain.KindResident}, ContactableOnly: true})
		if err != nil {
			return err
		}
		for _, r := range reps {
			l := lang.Pick(r.PreferredLanguage, w.DefaultLanguage)
			text := i18n.T("advisory", l, i18n.Vars{"ward": w.Name, "partner": e.partnerName(w), "how": i18n.T("how_to_report", l, nil)})
			adv := domain.Advisory{AlertID: a.ID, ReporterID: r.ID, Level: a.Level, Language: l, Channel: r.PreferredChannel, SentAt: e.now(), Text: text}
			fresh, err := tx.CreateAdvisory(ctx, &adv)
			if err != nil {
				return err
			}
			if !fresh {
				continue
			}
			r.PreferredLanguage = l
			mid, err := e.messageToReporter(ctx, tx, r, "advisory", text, false, "advisory", adv.ID, nil)
			if err != nil {
				return err
			}
			if err := tx.SetAdvisoryMessage(ctx, adv.ID, mid, false); err != nil {
				return err
			}
			sent++
		}
		e.event(ctx, tx, "advisory-agent", "advisory.sent", "alert", a.ID, a.WardID, map[string]any{"households": sent, "level": a.Level})
		return nil
	})
	return sent, err
}
