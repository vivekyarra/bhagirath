package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const alertCols = `id, ward_id, level, peak_level, rule_version, rule_sha256, triggered_at, updated_at, resolved_at, signals,
	reason_text, reason_local, status, advisory_approved_at, advisory_approved_by`

func scanAlert(r pgx.Row) (domain.Alert, error) {
	var a domain.Alert
	var lvl, peak string
	err := r.Scan(&a.ID, &a.WardID, &lvl, &peak, &a.RuleVersion, &a.RuleSHA256, &a.TriggeredAt, &a.UpdatedAt, &a.ResolvedAt,
		&a.Signals, &a.ReasonText, &a.ReasonLocal, &a.Status, &a.AdvisoryApprovedAt, &a.AdvisoryApprovedBy)
	if err != nil {
		return a, notFound(err)
	}
	a.Level, _ = domain.ParseLevel(lvl)
	a.PeakLevel, _ = domain.ParseLevel(peak)
	return a, nil
}

// ActiveAlert returns the ward's active alert, or ErrNotFound.
func (s *Store) ActiveAlert(ctx context.Context, wardID string) (domain.Alert, error) {
	return scanAlert(s.q.QueryRow(ctx, `SELECT `+alertCols+` FROM alert WHERE ward_id=$1 AND status='active' FOR UPDATE`, wardID))
}

// GetAlert loads one alert.
func (s *Store) GetAlert(ctx context.Context, id int64) (domain.Alert, error) {
	return scanAlert(s.q.QueryRow(ctx, `SELECT `+alertCols+` FROM alert WHERE id=$1`, id))
}

// CreateAlert inserts an alert.
func (s *Store) CreateAlert(ctx context.Context, a *domain.Alert) error {
	return s.q.QueryRow(ctx, `INSERT INTO alert(ward_id, level, peak_level, rule_version, rule_sha256, triggered_at, updated_at,
			signals, reason_text, reason_local, status)
		VALUES ($1,$2,$3,$4,$5,$6,$6,$7,$8,$9,'active') RETURNING id`,
		a.WardID, a.Level.String(), a.PeakLevel.String(), a.RuleVersion, a.RuleSHA256, a.TriggeredAt, a.Signals, a.ReasonText, a.ReasonLocal).Scan(&a.ID)
}

// UpdateAlert writes level, signals, reason and status.
func (s *Store) UpdateAlert(ctx context.Context, a *domain.Alert) error {
	_, err := s.q.Exec(ctx, `UPDATE alert SET level=$2, peak_level=$3, updated_at=$4, resolved_at=$5, signals=$6, reason_text=$7,
		reason_local=$8, status=$9, advisory_approved_at=$10, advisory_approved_by=$11 WHERE id=$1`,
		a.ID, a.Level.String(), a.PeakLevel.String(), a.UpdatedAt, a.ResolvedAt, a.Signals, a.ReasonText, a.ReasonLocal, a.Status,
		a.AdvisoryApprovedAt, a.AdvisoryApprovedBy)
	return err
}

// AddTransition records a level change.
func (s *Store) AddTransition(ctx context.Context, t domain.AlertTransition) error {
	_, err := s.q.Exec(ctx, `INSERT INTO alert_transition(alert_id, at, from_level, to_level, reason_text, signals) VALUES ($1,$2,$3,$4,$5,$6)`,
		t.AlertID, t.At, t.FromLevel.String(), t.ToLevel.String(), t.ReasonText, t.Signals)
	return err
}

// Transitions returns an alert's level history (oldest first).
func (s *Store) Transitions(ctx context.Context, alertID int64) ([]domain.AlertTransition, error) {
	rows, err := s.q.Query(ctx, `SELECT id, alert_id, at, from_level, to_level, reason_text, signals FROM alert_transition WHERE alert_id=$1 ORDER BY at, id`, alertID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AlertTransition, error) {
		var t domain.AlertTransition
		var from, to string
		err := r.Scan(&t.ID, &t.AlertID, &t.At, &from, &to, &t.ReasonText, &t.Signals)
		t.FromLevel, _ = domain.ParseLevel(from)
		t.ToLevel, _ = domain.ParseLevel(to)
		return t, err
	})
}

// AlertFilter narrows ListAlerts.
type AlertFilter struct {
	WardID string
	Status string
	Since  time.Time
	Limit  int
}

// ListAlerts returns alerts, most recently updated first.
func (s *Store) ListAlerts(ctx context.Context, f AlertFilter) ([]domain.Alert, error) {
	w := where{}
	if f.WardID != "" {
		w.add("ward_id = ?", f.WardID)
	}
	if f.Status != "" {
		w.add("status = ?", f.Status)
	}
	if !f.Since.IsZero() {
		w.add("triggered_at >= ?", f.Since)
	}
	rows, err := s.q.Query(ctx, `SELECT `+alertCols+` FROM alert`+w.sql()+` ORDER BY (status='active') DESC, updated_at DESC LIMIT `+itoa(clampLimit(f.Limit, 200, 10000)), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AllTransitions returns every transition (for metrics).
func (s *Store) AllTransitions(ctx context.Context) ([]domain.AlertTransition, error) {
	rows, err := s.q.Query(ctx, `SELECT id, alert_id, at, from_level, to_level, reason_text, '{}'::jsonb FROM alert_transition ORDER BY at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AlertTransition, error) {
		var t domain.AlertTransition
		var from, to string
		err := r.Scan(&t.ID, &t.AlertID, &t.At, &from, &to, &t.ReasonText, &t.Signals)
		t.FromLevel, _ = domain.ParseLevel(from)
		t.ToLevel, _ = domain.ParseLevel(to)
		return t, err
	})
}
