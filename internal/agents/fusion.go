package agents

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Snapshot loads everything the rule needs for one ward at time now.
func (e *Engine) Snapshot(ctx context.Context, st *store.Store, w domain.Ward, now time.Time) (rule.Snapshot, error) {
	since := now.Add(-time.Duration(e.Rule.LookbackDays()) * 24 * time.Hour)
	snap := rule.Snapshot{WardID: w.ID, WardName: w.Name}
	cs, err := st.RuleComplaints(ctx, w.ID, since)
	if err != nil {
		return snap, err
	}
	for _, c := range cs {
		snap.Complaints = append(snap.Complaints, rule.ComplaintSignal{ID: c.Ref, Reporter: c.ReporterRef, IssueType: c.IssueType, At: c.CreatedAt,
			Point: c.Point, Invalid: c.Status == domain.StatusInvalid, VerifiedFixedAt: c.VerifiedFixedAt})
	}
	ts, err := st.ListWaterTests(ctx, w.ID, since, 50000)
	if err != nil {
		return snap, err
	}
	for _, t := range ts {
		snap.Tests = append(snap.Tests, rule.TestSignal{ID: t.Ref, SampledAt: t.SampledAt, Parameter: t.Parameter, Contaminated: t.IsContaminated,
			Value: t.Value, Limit: t.LimitValue, Unit: t.Unit})
	}
	is, err := st.ListIllness(ctx, w.ID, since, 50000)
	if err != nil {
		return snap, err
	}
	for _, i := range is {
		snap.Illness = append(snap.Illness, rule.IllnessSignal{ID: i.Ref, Reporter: i.ReporterRef, At: i.ReportedAt, Diarrhoea: i.CasesDiarrhoea,
			Vomiting: i.CasesVomiting, ORS: i.ORSUnitsSold})
	}
	return snap, nil
}

func wardLockKey(id string) int64 {
	h := fnv.New64a()
	h.Write([]byte("ward:" + id))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// EvaluateWard runs the Fusion & Alert Agent for one ward and applies the
// consequences (new alert, level change, resolution) atomically.
func (e *Engine) EvaluateWard(ctx context.Context, wardID string) (rule.Result, error) {
	var res rule.Result
	w, err := e.Store.GetWard(ctx, wardID)
	if err != nil {
		return res, err
	}
	var alertForAdvisory *domain.Alert
	err = e.Store.Tx(ctx, func(tx *store.Store) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, wardLockKey(wardID)); err != nil {
			return err
		}
		now := e.now()
		snap, err := e.Snapshot(ctx, tx, w, now)
		if err != nil {
			return err
		}
		res = e.Rule.Evaluate(snap, now)
		signals, _ := json.Marshal(res)
		active, err := tx.ActiveAlert(ctx, wardID)
		hasActive := err == nil
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		switch {
		case !hasActive && res.Level == domain.LevelNone:
			return nil
		case !hasActive:
			a := domain.Alert{WardID: wardID, Level: res.Level, PeakLevel: res.Level, RuleVersion: res.RuleVersion, RuleSHA256: res.RuleSHA256,
				TriggeredAt: now, Signals: signals, ReasonText: res.Reason, ReasonLocal: res.ReasonHI, Status: "active"}
			if err := tx.CreateAlert(ctx, &a); err != nil {
				return err
			}
			if err := tx.AddTransition(ctx, domain.AlertTransition{AlertID: a.ID, At: now, FromLevel: domain.LevelNone, ToLevel: res.Level, ReasonText: res.Reason, Signals: signals}); err != nil {
				return err
			}
			e.event(ctx, tx, "fusion-agent", "alert.raised", "alert", a.ID, wardID, map[string]any{"level": res.Level, "reason": res.Reason})
			if err := e.EscalateAlert(ctx, tx, a, res.Level, "new_alert"); err != nil {
				return err
			}
			if res.Level >= domain.LevelWarning {
				alertForAdvisory = &a
			}
		case res.Level == domain.LevelNone:
			active.Status, active.ResolvedAt, active.UpdatedAt = "resolved", &now, now
			active.Signals = signals
			if err := tx.UpdateAlert(ctx, &active); err != nil {
				return err
			}
			if err := tx.AddTransition(ctx, domain.AlertTransition{AlertID: active.ID, At: now, FromLevel: active.Level, ToLevel: domain.LevelNone, ReasonText: "All signals cleared under rule " + res.RuleVersion, Signals: signals}); err != nil {
				return err
			}
			e.event(ctx, tx, "fusion-agent", "alert.resolved", "alert", active.ID, wardID, map[string]any{"from": active.Level})
			return e.closeAlertEscalations(ctx, tx, active.ID, now)
		default:
			prev := active.Level
			active.Level, active.Signals, active.ReasonText, active.ReasonLocal, active.UpdatedAt = res.Level, signals, res.Reason, res.ReasonHI, now
			raisedPeak := res.Level > active.PeakLevel
			if raisedPeak {
				active.PeakLevel = res.Level
			}
			if err := tx.UpdateAlert(ctx, &active); err != nil {
				return err
			}
			if prev != res.Level {
				if err := tx.AddTransition(ctx, domain.AlertTransition{AlertID: active.ID, At: now, FromLevel: prev, ToLevel: res.Level, ReasonText: res.Reason, Signals: signals}); err != nil {
					return err
				}
				e.event(ctx, tx, "fusion-agent", "alert.level_changed", "alert", active.ID, wardID, map[string]any{"from": prev, "to": res.Level, "reason": res.Reason})
			}
			if raisedPeak {
				if err := e.EscalateAlert(ctx, tx, active, res.Level, "level_up"); err != nil {
					return err
				}
				if res.Level >= domain.LevelWarning {
					alertForAdvisory = &active
				}
			}
		}
		return nil
	})
	if err != nil || alertForAdvisory == nil {
		return res, err
	}
	return res, e.OnAlertWarning(ctx, alertForAdvisory.ID)
}

// EvaluateAll runs the alert engine over every ward (scheduled every 15 min).
func (e *Engine) EvaluateAll(ctx context.Context) (map[string]rule.Result, error) {
	wards, err := e.Store.ListWards(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]rule.Result{}
	var firstErr error
	for _, w := range wards {
		r, err := e.EvaluateWard(ctx, w.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if e.Log != nil {
				e.Log.Error("ward evaluation failed", "ward", w.ID, "err", err)
			}
			continue
		}
		out[w.ID] = r
	}
	return out, firstErr
}
