// Package storetest gives each test an isolated, migrated Postgres schema.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/vivekyarra/bhagirath/internal/store"
)

// EnvVar names the database used by tests.
const EnvVar = "BHAGIRATH_TEST_DATABASE_URL"

// New returns a Store bound to a fresh schema that is dropped after the test.
// The test is skipped when no test database is configured, unless CI=true,
// where a missing database is a failure.
func New(t testing.TB) *store.Store {
	t.Helper()
	base := os.Getenv(EnvVar)
	if base == "" {
		if os.Getenv("CI") == "true" {
			t.Fatalf("%s must be set in CI", EnvVar)
		}
		t.Skipf("%s not set; skipping database test", EnvVar)
	}
	ctx := context.Background()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "t_" + hex.EncodeToString(b)

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		_ = admin.Close(context.Background())
	})
	return s
}
