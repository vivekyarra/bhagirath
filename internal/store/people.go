package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

// UpsertWard inserts or replaces a ward definition.
func (s *Store) UpsertWard(ctx context.Context, w domain.Ward) error {
	poly, err := json.Marshal(w.Polygon)
	if err != nil {
		return err
	}
	lm, err := json.Marshal(w.Landmarks)
	if err != nil {
		return err
	}
	_, err = s.q.Exec(ctx, `INSERT INTO ward(id, name, city_or_block, district, state, polygon, centroid_lat, centroid_lon,
			default_language, landmarks, partner_name, demo)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, city_or_block=EXCLUDED.city_or_block, district=EXCLUDED.district,
			state=EXCLUDED.state, polygon=EXCLUDED.polygon, centroid_lat=EXCLUDED.centroid_lat, centroid_lon=EXCLUDED.centroid_lon,
			default_language=EXCLUDED.default_language, landmarks=EXCLUDED.landmarks, partner_name=EXCLUDED.partner_name, demo=EXCLUDED.demo`,
		w.ID, w.Name, w.CityOrBlock, w.District, w.State, poly, w.Centroid.Lat, w.Centroid.Lon, w.DefaultLanguage, lm, w.PartnerName, w.Demo)
	return err
}

const wardCols = `id, name, city_or_block, district, state, polygon, centroid_lat, centroid_lon, default_language, landmarks, partner_name, demo`

func scanWard(r pgx.Row) (domain.Ward, error) {
	var w domain.Ward
	var poly, lm []byte
	if err := r.Scan(&w.ID, &w.Name, &w.CityOrBlock, &w.District, &w.State, &poly, &w.Centroid.Lat, &w.Centroid.Lon,
		&w.DefaultLanguage, &lm, &w.PartnerName, &w.Demo); err != nil {
		return w, notFound(err)
	}
	if err := json.Unmarshal(poly, &w.Polygon); err != nil {
		return w, err
	}
	if err := json.Unmarshal(lm, &w.Landmarks); err != nil {
		return w, err
	}
	return w, nil
}

// ListWards returns all wards ordered by name.
func (s *Store) ListWards(ctx context.Context) ([]domain.Ward, error) {
	rows, err := s.q.Query(ctx, `SELECT `+wardCols+` FROM ward ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Ward
	for rows.Next() {
		w, err := scanWard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWard loads one ward.
func (s *Store) GetWard(ctx context.Context, id string) (domain.Ward, error) {
	return scanWard(s.q.QueryRow(ctx, `SELECT `+wardCols+` FROM ward WHERE id=$1`, id))
}

const reporterCols = `id, ref, COALESCE(phone_hash,''), phone_enc, kind, label, preferred_language, preferred_channel,
	COALESCE(ward_id,''), home_lat, home_lon, has_smartphone, consent_at, consent_asked_at, consent_declined, created_at`

func scanReporter(r pgx.Row) (domain.Reporter, error) {
	var rp domain.Reporter
	var lat, lon *float64
	err := r.Scan(&rp.ID, &rp.Ref, &rp.PhoneHash, &rp.PhoneEnc, &rp.Kind, &rp.Label, &rp.PreferredLanguage, &rp.PreferredChannel,
		&rp.WardID, &lat, &lon, &rp.HasSmartphone, &rp.ConsentAt, &rp.ConsentAskedAt, &rp.ConsentDeclined, &rp.CreatedAt)
	rp.Home = toPoint(lat, lon)
	return rp, notFound(err)
}

// CreateReporter inserts a reporter. If the phone hash already exists the
// existing row is returned with created=false.
func (s *Store) CreateReporter(ctx context.Context, rp domain.Reporter) (domain.Reporter, bool, error) {
	lat, lon := pointArgs(rp.Home)
	row := s.q.QueryRow(ctx, `INSERT INTO reporter(ref, phone_hash, phone_enc, kind, label, preferred_language, preferred_channel,
			ward_id, home_lat, home_lon, has_smartphone, consent_at, consent_asked_at, consent_declined, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (phone_hash) DO NOTHING
		RETURNING `+reporterCols,
		rp.Ref, nullStr(rp.PhoneHash), rp.PhoneEnc, rp.Kind, rp.Label, rp.PreferredLanguage, defaultStr(rp.PreferredChannel, "voice"),
		nullStr(rp.WardID), lat, lon, rp.HasSmartphone, rp.ConsentAt, rp.ConsentAskedAt, rp.ConsentDeclined, rp.CreatedAt)
	out, err := scanReporter(row)
	if errors.Is(err, ErrNotFound) && rp.PhoneHash != "" {
		existing, err := s.GetReporterByHash(ctx, rp.PhoneHash)
		return existing, false, err
	}
	return out, err == nil, err
}

// GetReporter loads a reporter by id.
func (s *Store) GetReporter(ctx context.Context, id int64) (domain.Reporter, error) {
	return scanReporter(s.q.QueryRow(ctx, `SELECT `+reporterCols+` FROM reporter WHERE id=$1`, id))
}

// GetReporterByHash loads a reporter by phone hash.
func (s *Store) GetReporterByHash(ctx context.Context, hash string) (domain.Reporter, error) {
	return scanReporter(s.q.QueryRow(ctx, `SELECT `+reporterCols+` FROM reporter WHERE phone_hash=$1`, hash))
}

// GetReporterByRef loads a reporter by public ref.
func (s *Store) GetReporterByRef(ctx context.Context, ref string) (domain.Reporter, error) {
	return scanReporter(s.q.QueryRow(ctx, `SELECT `+reporterCols+` FROM reporter WHERE ref=$1`, ref))
}

// UpdateReporter writes mutable reporter fields.
func (s *Store) UpdateReporter(ctx context.Context, rp domain.Reporter) error {
	lat, lon := pointArgs(rp.Home)
	_, err := s.q.Exec(ctx, `UPDATE reporter SET phone_enc=$2, kind=$3, label=$4, preferred_language=$5, preferred_channel=$6,
		ward_id=$7, home_lat=$8, home_lon=$9, has_smartphone=$10, consent_at=$11, consent_asked_at=$12, consent_declined=$13
		WHERE id=$1`, rp.ID, rp.PhoneEnc, rp.Kind, rp.Label, rp.PreferredLanguage, defaultStr(rp.PreferredChannel, "voice"),
		nullStr(rp.WardID), lat, lon, rp.HasSmartphone, rp.ConsentAt, rp.ConsentAskedAt, rp.ConsentDeclined)
	return err
}

// ReporterFilter narrows ListReporters.
type ReporterFilter struct {
	WardID          string
	Kinds           []domain.ReporterKind
	ContactableOnly bool
	Limit           int
}

// ListReporters returns reporters matching f.
func (s *Store) ListReporters(ctx context.Context, f ReporterFilter) ([]domain.Reporter, error) {
	w := where{}
	if f.WardID != "" {
		w.add("ward_id = ?", f.WardID)
	}
	if len(f.Kinds) > 0 {
		ks := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			ks[i] = string(k)
		}
		w.add("kind = ANY(?)", ks)
	}
	if f.ContactableOnly {
		w.add("consent_at IS NOT NULL AND phone_enc IS NOT NULL")
	}
	limit := clampLimit(f.Limit, 10000, 100000)
	rows, err := s.q.Query(ctx, `SELECT `+reporterCols+` FROM reporter`+w.sql()+` ORDER BY id LIMIT `+itoa(limit), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Reporter
	for rows.Next() {
		rp, err := scanReporter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

// PurgeUnconsentedPhones deletes encrypted numbers of reporters who declined
// consent or never answered the consent prompt before cutoff. The keyed hash
// is kept so repeat complaints still count as the same household.
func (s *Store) PurgeUnconsentedPhones(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `UPDATE reporter SET phone_enc=NULL
		WHERE phone_enc IS NOT NULL AND consent_at IS NULL AND kind='resident'
		  AND (consent_declined OR COALESCE(consent_asked_at, created_at) < $1)`, cutoff)
	return tag.RowsAffected(), err
}

// SaveMedia stores a blob and returns its id.
func (s *Store) SaveMedia(ctx context.Context, m domain.Media) (int64, error) {
	var id int64
	err := s.q.QueryRow(ctx, `INSERT INTO media(sha256, content_type, size, data, kind, created_at, delete_after)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, m.SHA256, m.ContentType, len(m.Data), m.Data, m.Kind, m.CreatedAt, m.DeleteAfter).Scan(&id)
	return id, err
}

// GetMedia loads a blob.
func (s *Store) GetMedia(ctx context.Context, id int64) (domain.Media, error) {
	var m domain.Media
	err := s.q.QueryRow(ctx, `SELECT id, sha256, content_type, size, COALESCE(data, ''::bytea), kind, created_at, delete_after FROM media WHERE id=$1`, id).
		Scan(&m.ID, &m.SHA256, &m.ContentType, &m.Size, &m.Data, &m.Kind, &m.CreatedAt, &m.DeleteAfter)
	return m, notFound(err)
}

// PurgeExpiredMedia drops blob bytes past their retention date, keeping the
// metadata row (hash, size) for audit.
func (s *Store) PurgeExpiredMedia(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `UPDATE media SET data=NULL WHERE data IS NOT NULL AND delete_after IS NOT NULL AND delete_after < $1`, now)
	return tag.RowsAffected(), err
}

// Conversation is pending multi-step state for a reporter.
type Conversation struct {
	ReporterID  int64
	State       string
	ComplaintID *int64
	ClosureID   *int64
	ExpiresAt   time.Time
}

// SetConversation stores conversation state.
func (s *Store) SetConversation(ctx context.Context, c Conversation, now time.Time) error {
	_, err := s.q.Exec(ctx, `INSERT INTO conversation(reporter_id, state, complaint_id, closure_id, expires_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (reporter_id) DO UPDATE SET state=EXCLUDED.state, complaint_id=EXCLUDED.complaint_id,
		closure_id=EXCLUDED.closure_id, expires_at=EXCLUDED.expires_at, updated_at=EXCLUDED.updated_at`,
		c.ReporterID, c.State, c.ComplaintID, c.ClosureID, c.ExpiresAt, now)
	return err
}

// GetConversation returns unexpired state, or ErrNotFound.
func (s *Store) GetConversation(ctx context.Context, reporterID int64, now time.Time) (Conversation, error) {
	var c Conversation
	err := s.q.QueryRow(ctx, `SELECT reporter_id, state, complaint_id, closure_id, expires_at FROM conversation
		WHERE reporter_id=$1 AND expires_at > $2`, reporterID, now).Scan(&c.ReporterID, &c.State, &c.ComplaintID, &c.ClosureID, &c.ExpiresAt)
	return c, notFound(err)
}

// ClearConversation removes state.
func (s *Store) ClearConversation(ctx context.Context, reporterID int64) error {
	_, err := s.q.Exec(ctx, `DELETE FROM conversation WHERE reporter_id=$1`, reporterID)
	return err
}

// UpsertUser creates or updates a dashboard user.
func (s *Store) UpsertUser(ctx context.Context, u domain.User) error {
	_, err := s.q.Exec(ctx, `INSERT INTO app_user(username, password_hash, role, created_at) VALUES ($1,$2,$3,$4)
		ON CONFLICT (username) DO UPDATE SET password_hash=EXCLUDED.password_hash, role=EXCLUDED.role`, u.Username, u.PasswordHash, u.Role, u.CreatedAt)
	return err
}

// GetUser loads a dashboard user.
func (s *Store) GetUser(ctx context.Context, username string) (domain.User, error) {
	var u domain.User
	err := s.q.QueryRow(ctx, `SELECT id, username, password_hash, role, created_at FROM app_user WHERE username=$1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	return u, notFound(err)
}

// CreateOutreach logs households informed.
func (s *Store) CreateOutreach(ctx context.Context, o domain.Outreach) error {
	_, err := s.q.Exec(ctx, `INSERT INTO outreach(ward_id, date, households, method, note, logged_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, o.WardID, o.Date, o.Households, o.Method, o.Note, o.LoggedBy, o.CreatedAt)
	return err
}

// ListOutreach returns outreach logs, newest first.
func (s *Store) ListOutreach(ctx context.Context) ([]domain.Outreach, error) {
	rows, err := s.q.Query(ctx, `SELECT id, ward_id, date, households, method, note, logged_by, created_at FROM outreach ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Outreach, error) {
		var o domain.Outreach
		err := r.Scan(&o.ID, &o.WardID, &o.Date, &o.Households, &o.Method, &o.Note, &o.LoggedBy, &o.CreatedAt)
		return o, err
	})
}

// CreateBaseline records a pre-Bhagirath time-to-fix observation.
func (s *Store) CreateBaseline(ctx context.Context, b domain.Baseline) error {
	_, err := s.q.Exec(ctx, `INSERT INTO baseline(ward_id, source, days_to_fix, resolved, note, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		b.WardID, b.Source, b.DaysToFix, b.Resolved, b.Note, b.CreatedAt)
	return err
}

// ListBaselines returns all baseline observations.
func (s *Store) ListBaselines(ctx context.Context) ([]domain.Baseline, error) {
	rows, err := s.q.Query(ctx, `SELECT id, ward_id, source, days_to_fix, resolved, note, created_at FROM baseline ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Baseline, error) {
		var b domain.Baseline
		err := r.Scan(&b.ID, &b.WardID, &b.Source, &b.DaysToFix, &b.Resolved, &b.Note, &b.CreatedAt)
		return b, err
	})
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func itoa(n int) string { return strconv.Itoa(n) }
