package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const officialCols = `id, name, role, department, ward_ids, phone_enc, email, ladder_level, grievance_channel, preferred_channel, language, active`

func scanOfficial(r pgx.Row) (domain.Official, error) {
	var o domain.Official
	err := r.Scan(&o.ID, &o.Name, &o.Role, &o.Department, &o.WardIDs, &o.PhoneEnc, &o.Email, &o.LadderLevel, &o.GrievanceChannel,
		&o.PreferredChannel, &o.Language, &o.Active)
	return o, notFound(err)
}

// CreateOfficial inserts an official.
func (s *Store) CreateOfficial(ctx context.Context, o *domain.Official) error {
	return s.q.QueryRow(ctx, `INSERT INTO official(name, role, department, ward_ids, phone_enc, email, ladder_level, grievance_channel,
		preferred_channel, language, active) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		o.Name, o.Role, o.Department, o.WardIDs, o.PhoneEnc, o.Email, o.LadderLevel, o.GrievanceChannel, defaultStr(o.PreferredChannel, "email"),
		defaultStr(o.Language, "en"), o.Active).Scan(&o.ID)
}

// UpdateOfficial writes all fields.
func (s *Store) UpdateOfficial(ctx context.Context, o domain.Official) error {
	_, err := s.q.Exec(ctx, `UPDATE official SET name=$2, role=$3, department=$4, ward_ids=$5, phone_enc=$6, email=$7, ladder_level=$8,
		grievance_channel=$9, preferred_channel=$10, language=$11, active=$12 WHERE id=$1`,
		o.ID, o.Name, o.Role, o.Department, o.WardIDs, o.PhoneEnc, o.Email, o.LadderLevel, o.GrievanceChannel, o.PreferredChannel, o.Language, o.Active)
	return err
}

// GetOfficial loads one official.
func (s *Store) GetOfficial(ctx context.Context, id int64) (domain.Official, error) {
	return scanOfficial(s.q.QueryRow(ctx, `SELECT `+officialCols+` FROM official WHERE id=$1`, id))
}

// ListOfficials returns officials, optionally for one ward.
func (s *Store) ListOfficials(ctx context.Context, wardID string) ([]domain.Official, error) {
	w := where{}
	if wardID != "" {
		w.add("? = ANY(ward_ids)", wardID)
	}
	rows, err := s.q.Query(ctx, `SELECT `+officialCols+` FROM official`+w.sql()+` ORDER BY ladder_level, id`, w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Official
	for rows.Next() {
		o, err := scanOfficial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OfficialsAt returns active officials for a ward at a ladder rung.
func (s *Store) OfficialsAt(ctx context.Context, wardID string, level domain.LadderLevel) ([]domain.Official, error) {
	rows, err := s.q.Query(ctx, `SELECT `+officialCols+` FROM official WHERE active AND $1 = ANY(ward_ids) AND ladder_level=$2 ORDER BY id`, wardID, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Official
	for rows.Next() {
		o, err := scanOfficial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

const escalationCols = `id, ref, alert_id, complaint_id, official_id, level, ward_id, sent_at, channel, token_hash, deadline_at,
	acknowledged_at, ack_via, grievance_number, outcome, parent_id, reason, closed_at`

func scanEscalation(r pgx.Row) (domain.Escalation, error) {
	var e domain.Escalation
	err := r.Scan(&e.ID, &e.Ref, &e.AlertID, &e.ComplaintID, &e.OfficialID, &e.Level, &e.WardID, &e.SentAt, &e.Channel, &e.TokenHash,
		&e.DeadlineAt, &e.AcknowledgedAt, &e.AckVia, &e.GrievanceNumber, &e.Outcome, &e.ParentID, &e.Reason, &e.ClosedAt)
	return e, notFound(err)
}

func collectEscalations(rows pgx.Rows, err error) ([]domain.Escalation, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Escalation
	for rows.Next() {
		e, err := scanEscalation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CreateEscalation inserts an escalation and fills ID/Ref.
func (s *Store) CreateEscalation(ctx context.Context, e *domain.Escalation) error {
	if e.Ref == "" {
		ref, err := s.NextRef(ctx, "escalation_ref_seq", "ESC")
		if err != nil {
			return err
		}
		e.Ref = ref
	}
	return s.q.QueryRow(ctx, `INSERT INTO escalation(ref, alert_id, complaint_id, official_id, level, ward_id, sent_at, channel, token_hash,
			deadline_at, outcome, parent_id, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		e.Ref, e.AlertID, e.ComplaintID, e.OfficialID, e.Level, e.WardID, e.SentAt, e.Channel, e.TokenHash, e.DeadlineAt, e.Outcome,
		e.ParentID, e.Reason).Scan(&e.ID)
}

// UpdateEscalation writes mutable fields.
func (s *Store) UpdateEscalation(ctx context.Context, e domain.Escalation) error {
	_, err := s.q.Exec(ctx, `UPDATE escalation SET acknowledged_at=$2, ack_via=$3, grievance_number=$4, outcome=$5, closed_at=$6, channel=$7,
		deadline_at=$8 WHERE id=$1`, e.ID, e.AcknowledgedAt, e.AckVia, e.GrievanceNumber, e.Outcome, e.ClosedAt, e.Channel, e.DeadlineAt)
	return err
}

// GetEscalation loads one escalation.
func (s *Store) GetEscalation(ctx context.Context, id int64) (domain.Escalation, error) {
	return scanEscalation(s.q.QueryRow(ctx, `SELECT `+escalationCols+` FROM escalation WHERE id=$1`, id))
}

// GetEscalationByRef loads one escalation by reference.
func (s *Store) GetEscalationByRef(ctx context.Context, ref string) (domain.Escalation, error) {
	return scanEscalation(s.q.QueryRow(ctx, `SELECT `+escalationCols+` FROM escalation WHERE ref=$1`, ref))
}

// GetEscalationByToken loads the escalation an official's link belongs to.
func (s *Store) GetEscalationByToken(ctx context.Context, tokenHash string) (domain.Escalation, error) {
	return scanEscalation(s.q.QueryRow(ctx, `SELECT `+escalationCols+` FROM escalation WHERE token_hash=$1`, tokenHash))
}

// EscalationFilter narrows ListEscalations.
type EscalationFilter struct {
	AlertID     int64
	ComplaintID int64
	WardID      string
	Outcomes    []domain.EscalationOutcome
	Limit       int
}

// ListEscalations returns escalations, most urgent first.
func (s *Store) ListEscalations(ctx context.Context, f EscalationFilter) ([]domain.Escalation, error) {
	w := where{}
	if f.AlertID != 0 {
		w.add("alert_id = ?", f.AlertID)
	}
	if f.ComplaintID != 0 {
		w.add("complaint_id = ?", f.ComplaintID)
	}
	if f.WardID != "" {
		w.add("ward_id = ?", f.WardID)
	}
	if len(f.Outcomes) > 0 {
		os := make([]string, len(f.Outcomes))
		for i, o := range f.Outcomes {
			os[i] = string(o)
		}
		w.add("outcome = ANY(?)", os)
	}
	return collectEscalations(s.q.Query(ctx, `SELECT `+escalationCols+` FROM escalation`+w.sql()+
		` ORDER BY (outcome='pending') DESC, deadline_at ASC, id DESC LIMIT `+itoa(clampLimit(f.Limit, 500, 50000)), w.args...))
}

// DueEscalations locks escalations whose timer has expired: pending ones
// (no acknowledgement in time) and acknowledged complaint escalations whose
// fix window has passed ("acknowledged but never fixed").
func (s *Store) DueEscalations(ctx context.Context, now time.Time, limit int) ([]domain.Escalation, error) {
	return collectEscalations(s.q.Query(ctx, `SELECT `+escalationCols+` FROM escalation
		WHERE (outcome='pending' OR (outcome='acknowledged' AND complaint_id IS NOT NULL)) AND deadline_at <= $1
		ORDER BY deadline_at LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit))
}

// LockEscalation loads one escalation FOR UPDATE.
func (s *Store) LockEscalation(ctx context.Context, id int64) (domain.Escalation, error) {
	return scanEscalation(s.q.QueryRow(ctx, `SELECT `+escalationCols+` FROM escalation WHERE id=$1 FOR UPDATE`, id))
}

const closureCols = `id, complaint_id, claimed_at, claimed_by, escalation_id, evidence_type, evidence_media_id, retest_id, note,
	callback_attempts, next_callback_at, callback_at, callback_result, callback_channel, verified, decided_at`

func scanClosure(r pgx.Row) (domain.Closure, error) {
	var c domain.Closure
	err := r.Scan(&c.ID, &c.ComplaintID, &c.ClaimedAt, &c.ClaimedBy, &c.EscalationID, &c.EvidenceType, &c.EvidenceMediaID, &c.RetestID,
		&c.Note, &c.CallbackAttempts, &c.NextCallbackAt, &c.CallbackAt, &c.CallbackResult, &c.CallbackChannel, &c.Verified, &c.DecidedAt)
	return c, notFound(err)
}

func collectClosures(rows pgx.Rows, err error) ([]domain.Closure, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Closure
	for rows.Next() {
		c, err := scanClosure(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateClosure inserts a fix claim.
func (s *Store) CreateClosure(ctx context.Context, c *domain.Closure) error {
	return s.q.QueryRow(ctx, `INSERT INTO closure(complaint_id, claimed_at, claimed_by, escalation_id, evidence_type, evidence_media_id,
			retest_id, note, next_callback_at, callback_result)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending') RETURNING id`,
		c.ComplaintID, c.ClaimedAt, c.ClaimedBy, c.EscalationID, c.EvidenceType, c.EvidenceMediaID, c.RetestID, c.Note, c.NextCallbackAt).Scan(&c.ID)
}

// UpdateClosure writes verification state.
func (s *Store) UpdateClosure(ctx context.Context, c domain.Closure) error {
	_, err := s.q.Exec(ctx, `UPDATE closure SET callback_attempts=$2, next_callback_at=$3, callback_at=$4, callback_result=$5,
		callback_channel=$6, verified=$7, decided_at=$8 WHERE id=$1`,
		c.ID, c.CallbackAttempts, c.NextCallbackAt, c.CallbackAt, c.CallbackResult, c.CallbackChannel, c.Verified, c.DecidedAt)
	return err
}

// GetClosure loads one closure.
func (s *Store) GetClosure(ctx context.Context, id int64) (domain.Closure, error) {
	return scanClosure(s.q.QueryRow(ctx, `SELECT `+closureCols+` FROM closure WHERE id=$1`, id))
}

// LockClosure loads one closure FOR UPDATE.
func (s *Store) LockClosure(ctx context.Context, id int64) (domain.Closure, error) {
	return scanClosure(s.q.QueryRow(ctx, `SELECT `+closureCols+` FROM closure WHERE id=$1 FOR UPDATE`, id))
}

// PendingClosure returns the undecided closure for a complaint.
func (s *Store) PendingClosure(ctx context.Context, complaintID int64) (domain.Closure, error) {
	return scanClosure(s.q.QueryRow(ctx, `SELECT `+closureCols+` FROM closure WHERE complaint_id=$1 AND decided_at IS NULL`, complaintID))
}

// ClosuresFor returns every closure for a complaint (oldest first).
func (s *Store) ClosuresFor(ctx context.Context, complaintID int64) ([]domain.Closure, error) {
	return collectClosures(s.q.Query(ctx, `SELECT `+closureCols+` FROM closure WHERE complaint_id=$1 ORDER BY claimed_at`, complaintID))
}

// ListClosures returns closures; pendingOnly limits to undecided ones.
func (s *Store) ListClosures(ctx context.Context, pendingOnly bool, limit int) ([]domain.Closure, error) {
	q := `SELECT ` + closureCols + ` FROM closure`
	if pendingOnly {
		q += ` WHERE decided_at IS NULL`
	}
	return collectClosures(s.q.Query(ctx, q+` ORDER BY claimed_at DESC LIMIT `+itoa(clampLimit(limit, 500, 50000))))
}

// DueCallbacks locks undecided closures whose next callback is due.
func (s *Store) DueCallbacks(ctx context.Context, now time.Time, limit int) ([]domain.Closure, error) {
	return collectClosures(s.q.Query(ctx, `SELECT `+closureCols+` FROM closure
		WHERE decided_at IS NULL AND next_callback_at IS NOT NULL AND next_callback_at <= $1
		ORDER BY next_callback_at LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit))
}
