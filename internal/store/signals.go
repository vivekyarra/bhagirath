package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const testCols = `id, ref, source, lat, lon, ward_id, sampled_at, parameter, value, unit, limit_value, is_contaminated,
	evidence_media, evidence_url, note, created_at, created_by`

func scanTest(r pgx.Row) (domain.WaterTest, error) {
	var t domain.WaterTest
	var lat, lon *float64
	err := r.Scan(&t.ID, &t.Ref, &t.Source, &lat, &lon, &t.WardID, &t.SampledAt, &t.Parameter, &t.Value, &t.Unit, &t.LimitValue,
		&t.IsContaminated, &t.EvidenceMedia, &t.EvidenceURL, &t.Note, &t.CreatedAt, &t.CreatedBy)
	t.Point = toPoint(lat, lon)
	return t, notFound(err)
}

// CreateWaterTest inserts a water test result.
func (s *Store) CreateWaterTest(ctx context.Context, t *domain.WaterTest) error {
	if t.Ref == "" {
		ref, err := s.NextRef(ctx, "test_ref_seq", "WT")
		if err != nil {
			return err
		}
		t.Ref = ref
	}
	lat, lon := pointArgs(t.Point)
	return s.q.QueryRow(ctx, `INSERT INTO water_test(ref, source, lat, lon, ward_id, sampled_at, parameter, value, unit, limit_value,
			is_contaminated, evidence_media, evidence_url, note, created_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
		t.Ref, t.Source, lat, lon, t.WardID, t.SampledAt, t.Parameter, t.Value, t.Unit, t.LimitValue, t.IsContaminated,
		t.EvidenceMedia, t.EvidenceURL, t.Note, t.CreatedAt, t.CreatedBy).Scan(&t.ID)
}

// GetWaterTest loads one test.
func (s *Store) GetWaterTest(ctx context.Context, id int64) (domain.WaterTest, error) {
	return scanTest(s.q.QueryRow(ctx, `SELECT `+testCols+` FROM water_test WHERE id=$1`, id))
}

// ListWaterTests returns tests for a ward (all wards if empty) sampled since t.
func (s *Store) ListWaterTests(ctx context.Context, wardID string, since time.Time, limit int) ([]domain.WaterTest, error) {
	w := where{}
	if wardID != "" {
		w.add("ward_id = ?", wardID)
	}
	if !since.IsZero() {
		w.add("sampled_at >= ?", since)
	}
	rows, err := s.q.Query(ctx, `SELECT `+testCols+` FROM water_test`+w.sql()+` ORDER BY sampled_at DESC, id DESC LIMIT `+itoa(clampLimit(limit, 500, 50000)), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WaterTest
	for rows.Next() {
		t, err := scanTest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

const illnessCols = `i.id, i.ref, i.reporter_type, i.reporter_id, COALESCE(r.ref,''), i.ward_id, i.lat, i.lon, i.reported_at,
	i.cases_diarrhoea, i.cases_vomiting, i.ors_units_sold, i.notes, i.complaint_id, i.created_at`

func scanIllness(row pgx.Row) (domain.IllnessReport, error) {
	var i domain.IllnessReport
	var lat, lon *float64
	err := row.Scan(&i.ID, &i.Ref, &i.ReporterType, &i.ReporterID, &i.ReporterRef, &i.WardID, &lat, &lon, &i.ReportedAt,
		&i.CasesDiarrhoea, &i.CasesVomiting, &i.ORSUnitsSold, &i.Notes, &i.ComplaintID, &i.CreatedAt)
	i.Point = toPoint(lat, lon)
	return i, notFound(err)
}

// CreateIllness inserts an aggregate illness report.
func (s *Store) CreateIllness(ctx context.Context, i *domain.IllnessReport) error {
	if i.Ref == "" {
		ref, err := s.NextRef(ctx, "illness_ref_seq", "IL")
		if err != nil {
			return err
		}
		i.Ref = ref
	}
	lat, lon := pointArgs(i.Point)
	return s.q.QueryRow(ctx, `INSERT INTO illness_report(ref, reporter_type, reporter_id, ward_id, lat, lon, reported_at,
			cases_diarrhoea, cases_vomiting, ors_units_sold, notes, complaint_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		i.Ref, i.ReporterType, i.ReporterID, i.WardID, lat, lon, i.ReportedAt, i.CasesDiarrhoea, i.CasesVomiting, i.ORSUnitsSold,
		i.Notes, i.ComplaintID, i.CreatedAt).Scan(&i.ID)
}

// ListIllness returns illness reports for a ward (all if empty) since t.
func (s *Store) ListIllness(ctx context.Context, wardID string, since time.Time, limit int) ([]domain.IllnessReport, error) {
	w := where{}
	if wardID != "" {
		w.add("i.ward_id = ?", wardID)
	}
	if !since.IsZero() {
		w.add("i.reported_at >= ?", since)
	}
	rows, err := s.q.Query(ctx, `SELECT `+illnessCols+` FROM illness_report i LEFT JOIN reporter r ON r.id=i.reporter_id`+w.sql()+
		` ORDER BY i.reported_at DESC, i.id DESC LIMIT `+itoa(clampLimit(limit, 500, 50000)), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IllnessReport
	for rows.Next() {
		i, err := scanIllness(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// IllnessForComplaint reports whether a complaint already produced an illness signal.
func (s *Store) IllnessForComplaint(ctx context.Context, complaintID int64) (bool, error) {
	var ok bool
	err := s.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM illness_report WHERE complaint_id=$1)`, complaintID).Scan(&ok)
	return ok, err
}
