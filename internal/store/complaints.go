package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const complaintCols = `c.id, c.ref, c.reporter_id, COALESCE(r.ref,''), c.channel, COALESCE(c.provider_msg_id,''), c.audio_media_id,
	c.transcript, c.transcript_asr, c.transcript_status, c.language, c.issue_type, c.severity, c.description_en,
	c.lat, c.lon, c.location_text, c.location_method, COALESCE(c.ward_id,''), c.status, c.duplicate_of, c.needs_followup,
	c.extraction, c.created_at, c.updated_at, c.verified_fixed_at, c.reopen_count`

const complaintFrom = ` FROM complaint c LEFT JOIN reporter r ON r.id = c.reporter_id`

func scanComplaint(row pgx.Row) (domain.Complaint, error) {
	var c domain.Complaint
	var lat, lon *float64
	err := row.Scan(&c.ID, &c.Ref, &c.ReporterID, &c.ReporterRef, &c.Channel, &c.ProviderMsgID, &c.AudioMediaID,
		&c.Transcript, &c.TranscriptASR, &c.TranscriptStatus, &c.Language, &c.IssueType, &c.Severity, &c.DescriptionEN,
		&lat, &lon, &c.LocationText, &c.LocationMethod, &c.WardID, &c.Status, &c.DuplicateOf, &c.NeedsFollowup,
		&c.Extraction, &c.CreatedAt, &c.UpdatedAt, &c.VerifiedFixedAt, &c.ReopenCount)
	c.Point = toPoint(lat, lon)
	return c, notFound(err)
}

// CreateComplaint inserts a complaint and fills ID/Ref.
func (s *Store) CreateComplaint(ctx context.Context, c *domain.Complaint) error {
	if c.Ref == "" {
		ref, err := s.NextRef(ctx, "complaint_ref_seq", "BG")
		if err != nil {
			return err
		}
		c.Ref = ref
	}
	if len(c.Extraction) == 0 {
		c.Extraction = json.RawMessage(`{}`)
	}
	if c.Severity == 0 {
		c.Severity = 1
	}
	lat, lon := pointArgs(c.Point)
	return s.q.QueryRow(ctx, `INSERT INTO complaint(ref, reporter_id, channel, provider_msg_id, audio_media_id, transcript, transcript_asr,
			transcript_status, language, issue_type, severity, description_en, lat, lon, location_text, location_method, ward_id,
			status, duplicate_of, needs_followup, extraction, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$22) RETURNING id`,
		c.Ref, c.ReporterID, c.Channel, nullStr(c.ProviderMsgID), c.AudioMediaID, c.Transcript, c.TranscriptASR,
		c.TranscriptStatus, c.Language, c.IssueType, c.Severity, c.DescriptionEN, lat, lon, c.LocationText, c.LocationMethod,
		nullStr(c.WardID), c.Status, c.DuplicateOf, c.NeedsFollowup, c.Extraction, c.CreatedAt).Scan(&c.ID)
}

// UpdateComplaint writes all mutable fields.
func (s *Store) UpdateComplaint(ctx context.Context, c *domain.Complaint, now time.Time) error {
	lat, lon := pointArgs(c.Point)
	c.UpdatedAt = now
	if len(c.Extraction) == 0 {
		c.Extraction = json.RawMessage(`{}`)
	}
	_, err := s.q.Exec(ctx, `UPDATE complaint SET transcript=$2, transcript_asr=$3, transcript_status=$4, language=$5, issue_type=$6,
		severity=$7, description_en=$8, lat=$9, lon=$10, location_text=$11, location_method=$12, ward_id=$13, status=$14,
		duplicate_of=$15, needs_followup=$16, extraction=$17, updated_at=$18, verified_fixed_at=$19, reopen_count=$20, audio_media_id=$21
		WHERE id=$1`, c.ID, c.Transcript, c.TranscriptASR, c.TranscriptStatus, c.Language, c.IssueType, c.Severity, c.DescriptionEN,
		lat, lon, c.LocationText, c.LocationMethod, nullStr(c.WardID), c.Status, c.DuplicateOf, c.NeedsFollowup, c.Extraction,
		now, c.VerifiedFixedAt, c.ReopenCount, c.AudioMediaID)
	return err
}

// GetComplaint loads a complaint by id.
func (s *Store) GetComplaint(ctx context.Context, id int64) (domain.Complaint, error) {
	return scanComplaint(s.q.QueryRow(ctx, `SELECT `+complaintCols+complaintFrom+` WHERE c.id=$1`, id))
}

// GetComplaintByRef loads a complaint by public reference.
func (s *Store) GetComplaintByRef(ctx context.Context, ref string) (domain.Complaint, error) {
	return scanComplaint(s.q.QueryRow(ctx, `SELECT `+complaintCols+complaintFrom+` WHERE c.ref=$1`, ref))
}

// LockComplaint loads a complaint FOR UPDATE (must be inside Tx).
func (s *Store) LockComplaint(ctx context.Context, id int64) (domain.Complaint, error) {
	return scanComplaint(s.q.QueryRow(ctx, `SELECT `+complaintCols+complaintFrom+` WHERE c.id=$1 FOR UPDATE OF c`, id))
}

// ComplaintFilter narrows ListComplaints.
type ComplaintFilter struct {
	WardID            string
	Statuses          []domain.ComplaintStatus
	Channel           domain.Channel
	ReporterID        int64
	TranscriptStatus  domain.TranscriptStatus
	NeedsFollowup     *bool
	Since             time.Time
	Until             time.Time
	IncludeDuplicates bool
	Search            string
	Limit             int
	Offset            int
}

// ListComplaints returns complaints, newest first.
func (s *Store) ListComplaints(ctx context.Context, f ComplaintFilter) ([]domain.Complaint, error) {
	w := complaintWhere(f)
	limit := clampLimit(f.Limit, 200, 20000)
	rows, err := s.q.Query(ctx, `SELECT `+complaintCols+complaintFrom+w.sql()+` ORDER BY c.created_at DESC, c.id DESC LIMIT `+itoa(limit)+` OFFSET `+itoa(max(f.Offset, 0)), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Complaint
	for rows.Next() {
		c, err := scanComplaint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountComplaints counts complaints matching f.
func (s *Store) CountComplaints(ctx context.Context, f ComplaintFilter) (int, error) {
	w := complaintWhere(f)
	var n int
	err := s.q.QueryRow(ctx, `SELECT count(*)`+complaintFrom+w.sql(), w.args...).Scan(&n)
	return n, err
}

func complaintWhere(f ComplaintFilter) where {
	w := where{}
	if f.WardID != "" {
		w.add("c.ward_id = ?", f.WardID)
	}
	if len(f.Statuses) > 0 {
		ss := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			ss[i] = string(st)
		}
		w.add("c.status = ANY(?)", ss)
	}
	if f.Channel != "" {
		w.add("c.channel = ?", string(f.Channel))
	}
	if f.ReporterID != 0 {
		w.add("c.reporter_id = ?", f.ReporterID)
	}
	if f.TranscriptStatus != "" {
		w.add("c.transcript_status = ?", string(f.TranscriptStatus))
	}
	if f.NeedsFollowup != nil {
		w.add("c.needs_followup = ?", *f.NeedsFollowup)
	}
	if !f.Since.IsZero() {
		w.add("c.created_at >= ?", f.Since)
	}
	if !f.Until.IsZero() {
		w.add("c.created_at < ?", f.Until)
	}
	if !f.IncludeDuplicates {
		w.add("c.duplicate_of IS NULL")
	}
	if f.Search != "" {
		w.add("(c.ref ILIKE ? OR c.transcript ILIKE ? OR c.location_text ILIKE ?)", "%"+f.Search+"%", "%"+f.Search+"%", "%"+f.Search+"%")
	}
	return w
}

// RuleComplaints returns every complaint the rule needs for a ward at time
// now: all created in the lookback window plus any older one still open.
func (s *Store) RuleComplaints(ctx context.Context, wardID string, since time.Time) ([]domain.Complaint, error) {
	rows, err := s.q.Query(ctx, `SELECT `+complaintCols+complaintFrom+`
		WHERE c.ward_id=$1 AND c.duplicate_of IS NULL
		  AND (c.created_at >= $2 OR c.status NOT IN ('verified_fixed','invalid'))
		ORDER BY c.created_at`, wardID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Complaint
	for rows.Next() {
		c, err := scanComplaint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RecentByReporter returns the reporter's non-duplicate complaints since t.
func (s *Store) RecentByReporter(ctx context.Context, reporterID int64, since time.Time) ([]domain.Complaint, error) {
	return s.ListComplaints(ctx, ComplaintFilter{ReporterID: reporterID, Since: since, Limit: 50})
}

// ComplaintsByIDs loads several complaints.
func (s *Store) ComplaintsByIDs(ctx context.Context, ids []int64) ([]domain.Complaint, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.Query(ctx, `SELECT `+complaintCols+complaintFrom+` WHERE c.id = ANY($1) ORDER BY c.created_at`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Complaint
	for rows.Next() {
		c, err := scanComplaint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
