package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const msgCols = `id, kind, channel, recipient_type, recipient_id, recipient_label, language, subject, body, link, interactive, status,
	attempts, provider_id, error, entity, entity_id, created_at, next_attempt_at, sent_at`

func scanMessage(r pgx.Row) (domain.OutboundMessage, error) {
	var m domain.OutboundMessage
	var inter []byte
	err := r.Scan(&m.ID, &m.Kind, &m.Channel, &m.RecipientType, &m.RecipientID, &m.RecipientLabel, &m.Language, &m.Subject, &m.Body,
		&m.Link, &inter, &m.Status, &m.Attempts, &m.ProviderID, &m.Error, &m.Entity, &m.EntityID, &m.CreatedAt, &m.NextAttemptAt, &m.SentAt)
	if len(inter) > 0 {
		m.Interactive = json.RawMessage(inter)
	}
	return m, notFound(err)
}

func collectMessages(rows pgx.Rows, err error) ([]domain.OutboundMessage, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OutboundMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EnqueueMessage inserts an outbound message.
func (s *Store) EnqueueMessage(ctx context.Context, m *domain.OutboundMessage) error {
	var inter any
	if len(m.Interactive) > 0 {
		inter = m.Interactive
	}
	return s.q.QueryRow(ctx, `INSERT INTO outbound_message(kind, channel, recipient_type, recipient_id, recipient_label, language, subject,
			body, link, interactive, status, attempts, entity, entity_id, created_at, next_attempt_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,0,$12,$13,$14,$15) RETURNING id`,
		m.Kind, m.Channel, m.RecipientType, m.RecipientID, m.RecipientLabel, m.Language, m.Subject, m.Body, m.Link, inter, m.Status,
		m.Entity, m.EntityID, m.CreatedAt, m.NextAttemptAt).Scan(&m.ID)
}

// DueMessages locks queued messages ready for delivery.
func (s *Store) DueMessages(ctx context.Context, now time.Time, limit int) ([]domain.OutboundMessage, error) {
	return collectMessages(s.q.Query(ctx, `SELECT `+msgCols+` FROM outbound_message WHERE status='queued' AND next_attempt_at <= $1
		ORDER BY next_attempt_at LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit))
}

// UpdateMessage writes delivery state.
func (s *Store) UpdateMessage(ctx context.Context, m domain.OutboundMessage) error {
	_, err := s.q.Exec(ctx, `UPDATE outbound_message SET status=$2, attempts=$3, provider_id=$4, error=$5, next_attempt_at=$6, sent_at=$7, link=$8
		WHERE id=$1`, m.ID, m.Status, m.Attempts, m.ProviderID, m.Error, m.NextAttemptAt, m.SentAt, m.Link)
	return err
}

// MarkDeliveredByProvider sets status=delivered for a provider message id.
func (s *Store) MarkDeliveredByProvider(ctx context.Context, providerID string) (int64, error) {
	tag, err := s.q.Exec(ctx, `UPDATE outbound_message SET status='delivered' WHERE provider_id=$1 AND status IN ('sent','queued')`, providerID)
	if err != nil {
		return 0, err
	}
	_, err = s.q.Exec(ctx, `UPDATE advisory SET delivered=TRUE WHERE message_id IN (SELECT id FROM outbound_message WHERE provider_id=$1)`, providerID)
	return tag.RowsAffected(), err
}

// MessageFilter narrows ListMessages.
type MessageFilter struct {
	Kind     string
	Entity   string
	EntityID int64
	Limit    int
}

// ListMessages returns the outbox, newest first.
func (s *Store) ListMessages(ctx context.Context, f MessageFilter) ([]domain.OutboundMessage, error) {
	w := where{}
	if f.Kind != "" {
		w.add("kind = ?", f.Kind)
	}
	if f.Entity != "" {
		w.add("entity = ?", f.Entity)
	}
	if f.EntityID != 0 {
		w.add("entity_id = ?", f.EntityID)
	}
	return collectMessages(s.q.Query(ctx, `SELECT `+msgCols+` FROM outbound_message`+w.sql()+` ORDER BY created_at DESC, id DESC LIMIT `+itoa(clampLimit(f.Limit, 200, 20000)), w.args...))
}

// GetMessage loads one outbound message.
func (s *Store) GetMessage(ctx context.Context, id int64) (domain.OutboundMessage, error) {
	return scanMessage(s.q.QueryRow(ctx, `SELECT `+msgCols+` FROM outbound_message WHERE id=$1`, id))
}

// CreateAdvisory inserts an advisory; it returns false if this household
// already received an advisory for this alert at this level.
func (s *Store) CreateAdvisory(ctx context.Context, a *domain.Advisory) (bool, error) {
	err := s.q.QueryRow(ctx, `INSERT INTO advisory(alert_id, reporter_id, level, language, channel, sent_at, delivered, message_id, text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (alert_id, reporter_id, level) DO NOTHING RETURNING id`,
		a.AlertID, a.ReporterID, a.Level.String(), a.Language, a.Channel, a.SentAt, a.Delivered, a.MessageID, a.Text).Scan(&a.ID)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// SetAdvisoryMessage links an advisory to its outbound message.
func (s *Store) SetAdvisoryMessage(ctx context.Context, advisoryID, messageID int64, delivered bool) error {
	_, err := s.q.Exec(ctx, `UPDATE advisory SET message_id=$2, delivered=$3 WHERE id=$1`, advisoryID, messageID, delivered)
	return err
}

// ListAdvisories returns advisories for an alert (all if 0).
func (s *Store) ListAdvisories(ctx context.Context, alertID int64) ([]domain.Advisory, error) {
	w := where{}
	if alertID != 0 {
		w.add("alert_id = ?", alertID)
	}
	rows, err := s.q.Query(ctx, `SELECT id, alert_id, reporter_id, level, language, channel, sent_at, delivered, message_id, text FROM advisory`+w.sql()+` ORDER BY sent_at DESC`, w.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Advisory, error) {
		var a domain.Advisory
		var lvl string
		err := r.Scan(&a.ID, &a.AlertID, &a.ReporterID, &lvl, &a.Language, &a.Channel, &a.SentAt, &a.Delivered, &a.MessageID, &a.Text)
		a.Level, _ = domain.ParseLevel(lvl)
		return a, err
	})
}
