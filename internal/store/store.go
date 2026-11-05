// Package store is Bhagirath's Postgres persistence layer.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/migrations"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store wraps a connection pool or an open transaction.
type Store struct {
	pool *pgxpool.Pool
	q    querier
	inTx bool
}

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns < 4 {
		cfg.MaxConns = 8
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool, q: pool}, nil
}

// OpenWithRetry retries Open with exponential backoff until ctx expires or
// the budget is spent; containers often start before their database is ready.
func OpenWithRetry(ctx context.Context, url string, budget time.Duration) (*Store, error) {
	deadline := time.Now().Add(budget)
	wait := 500 * time.Millisecond
	for {
		s, err := Open(ctx, url)
		if err == nil {
			return s, nil
		}
		if time.Now().Add(wait).After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, 5*time.Second)
	}
}

// Close releases the pool.
func (s *Store) Close() {
	if s.pool != nil && !s.inTx {
		s.pool.Close()
	}
}

// Ping checks database health.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Tx runs fn inside a transaction. Nested calls reuse the outer transaction.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	if s.inTx {
		return fn(s)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, q: tx, inTx: true})
	})
}

// Migrate applies pending migrations under an advisory lock so that several
// replicas starting together do not race.
func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(727274)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(727274)`) //nolint:errcheck
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migration (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migration WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migration(name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ResetAll deletes every row (demo reset and tests only).
func (s *Store) ResetAll(ctx context.Context) error {
	_, err := s.q.Exec(ctx, `TRUNCATE advisory, outbound_message, closure, escalation, official, alert_transition, alert,
		illness_report, water_test, conversation, complaint, reporter, media, event_log, outreach, baseline,
		inbound_seen, kv, ward RESTART IDENTITY CASCADE;
		ALTER SEQUENCE complaint_ref_seq RESTART; ALTER SEQUENCE escalation_ref_seq RESTART;
		ALTER SEQUENCE test_ref_seq RESTART; ALTER SEQUENCE illness_ref_seq RESTART;`)
	return err
}

// NextRef returns a human-friendly reference such as "BG-000123".
func (s *Store) NextRef(ctx context.Context, seq, prefix string) (string, error) {
	switch seq {
	case "complaint_ref_seq", "escalation_ref_seq", "test_ref_seq", "illness_ref_seq":
	default:
		return "", fmt.Errorf("unknown sequence %q", seq)
	}
	var n int64
	if err := s.q.QueryRow(ctx, `SELECT nextval('`+seq+`')`).Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%06d", prefix, n), nil
}

// LogEvent appends to the audit log.
func (s *Store) LogEvent(ctx context.Context, e domain.Event) error {
	if len(e.Details) == 0 {
		e.Details = json.RawMessage(`{}`)
	}
	_, err := s.q.Exec(ctx, `INSERT INTO event_log(at, actor, action, entity, entity_id, ward_id, details)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7)`, e.At, e.Actor, e.Action, e.Entity, e.EntityID, e.WardID, e.Details)
	return err
}

// EventFilter narrows ListEvents.
type EventFilter struct {
	Actions  []string
	Entity   string
	EntityID string
	WardID   string
	Limit    int
}

// ListEvents returns recent audit events, newest first.
func (s *Store) ListEvents(ctx context.Context, f EventFilter) ([]domain.Event, error) {
	w := where{}
	if len(f.Actions) > 0 {
		w.add("action = ANY(?)", f.Actions)
	}
	if f.Entity != "" {
		w.add("entity = ?", f.Entity)
	}
	if f.EntityID != "" {
		w.add("entity_id = ?", f.EntityID)
	}
	if f.WardID != "" {
		w.add("ward_id = ?", f.WardID)
	}
	limit := clampLimit(f.Limit, 100, 5000)
	rows, err := s.q.Query(ctx, `SELECT id, at, actor, action, entity, entity_id, COALESCE(ward_id,''), details
		FROM event_log`+w.sql()+fmt.Sprintf(` ORDER BY at DESC, id DESC LIMIT %d`, limit), w.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Event, error) {
		var e domain.Event
		err := r.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.Entity, &e.EntityID, &e.WardID, &e.Details)
		return e, err
	})
}

// CountEvents counts audit events with the given action.
func (s *Store) CountEvents(ctx context.Context, action string) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE action=$1`, action).Scan(&n)
	return n, err
}

// KVGet reads a persistent value.
func (s *Store) KVGet(ctx context.Context, key string) (string, error) {
	var v string
	err := s.q.QueryRow(ctx, `SELECT value FROM kv WHERE key=$1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

// KVSet writes a persistent value.
func (s *Store) KVSet(ctx context.Context, key, value string, now time.Time) error {
	_, err := s.q.Exec(ctx, `INSERT INTO kv(key, value, updated_at) VALUES ($1,$2,$3)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=EXCLUDED.updated_at`, key, value, now)
	return err
}

// MarkInboundSeen records a provider message id; it returns false if the id
// was already processed (webhook retry).
func (s *Store) MarkInboundSeen(ctx context.Context, provider, id string, now time.Time) (bool, error) {
	if id == "" {
		return true, nil
	}
	tag, err := s.q.Exec(ctx, `INSERT INTO inbound_seen(provider, message_id, at) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, provider, id, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// TryLock takes a transaction-scoped advisory lock (job leader election).
// It must be called inside Tx.
func (s *Store) TryLock(ctx context.Context, key int64) (bool, error) {
	var ok bool
	err := s.q.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, key).Scan(&ok)
	return ok, err
}

// where builds parameterised WHERE clauses; "?" placeholders become $n.
type where struct {
	parts []string
	args  []any
}

func (w *where) add(cond string, args ...any) {
	for _, a := range args {
		w.args = append(w.args, a)
		cond = strings.Replace(cond, "?", fmt.Sprintf("$%d", len(w.args)), 1)
	}
	w.parts = append(w.parts, cond)
}

func (w *where) sql() string {
	if len(w.parts) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.parts, " AND ")
}

func clampLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func pointArgs(p *domain.Point) (any, any) {
	if p == nil || !p.Valid() {
		return nil, nil
	}
	return p.Lat, p.Lon
}

func toPoint(lat, lon *float64) *domain.Point {
	if lat == nil || lon == nil {
		return nil
	}
	return &domain.Point{Lat: *lat, Lon: *lon}
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Exec runs a raw statement (used for advisory locks inside transactions).
func (s *Store) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.q.Exec(ctx, sql, args...)
}
