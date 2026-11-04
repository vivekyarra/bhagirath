package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
	"github.com/vivekyarra/bhagirath/internal/store/storetest"
)

var t0 = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

func ward(id string) domain.Ward {
	return domain.Ward{ID: id, Name: "Ward " + id, Polygon: [][]domain.Point{{{Lat: 22.7, Lon: 75.8}, {Lat: 22.71, Lon: 75.8}, {Lat: 22.71, Lon: 75.81}}},
		Centroid: domain.Point{Lat: 22.705, Lon: 75.805}, DefaultLanguage: "hi"}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := storetest.New(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestReporterDedupByPhoneHash(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if err := s.UpsertWard(ctx, ward("w1")); err != nil {
		t.Fatal(err)
	}
	r1, created, err := s.CreateReporter(ctx, domain.Reporter{Ref: "R-A", PhoneHash: "h1", Kind: domain.KindResident, PreferredLanguage: "hi", CreatedAt: t0})
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	r2, created, err := s.CreateReporter(ctx, domain.Reporter{Ref: "R-B", PhoneHash: "h1", Kind: domain.KindResident, CreatedAt: t0})
	if err != nil || created || r2.ID != r1.ID {
		t.Fatalf("dedupe failed: %+v %v %v", r2, created, err)
	}
}

func TestComplaintLifecycleAndRuleQuery(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	_ = s.UpsertWard(ctx, ward("w1"))
	old := domain.Complaint{Channel: domain.ChannelWeb, IssueType: domain.IssueDirtyWater, Status: domain.StatusOpen,
		LocationMethod: domain.LocWardOnly, WardID: "w1", TranscriptStatus: domain.TranscriptNone, CreatedAt: t0.Add(-60 * 24 * time.Hour)}
	if err := s.CreateComplaint(ctx, &old); err != nil {
		t.Fatal(err)
	}
	fixed := old
	fixed.ID, fixed.Ref, fixed.Status = 0, "", domain.StatusVerifiedFixed
	if err := s.CreateComplaint(ctx, &fixed); err != nil {
		t.Fatal(err)
	}
	recent := old
	recent.ID, recent.Ref, recent.CreatedAt = 0, "", t0
	recent.Point = &domain.Point{Lat: 22.705, Lon: 75.805}
	if err := s.CreateComplaint(ctx, &recent); err != nil {
		t.Fatal(err)
	}
	got, err := s.RuleComplaints(ctx, "w1", t0.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rule complaints = %d, want 2 (old open + recent)", len(got))
	}
	if got[1].Point == nil || got[1].Point.Lat != 22.705 {
		t.Fatalf("point not round-tripped: %+v", got[1].Point)
	}
	c, err := s.GetComplaintByRef(ctx, recent.Ref)
	if err != nil || c.ID != recent.ID {
		t.Fatalf("by ref: %v", err)
	}
	if _, err := s.GetComplaint(ctx, 99999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing complaint err = %v", err)
	}
}

func TestProviderMessageIdempotency(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	ok, err := s.MarkInboundSeen(ctx, "whatsapp", "wamid.1", t0)
	if err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	ok, err = s.MarkInboundSeen(ctx, "whatsapp", "wamid.1", t0)
	if err != nil || ok {
		t.Fatalf("retry accepted: %v %v", ok, err)
	}
}

func TestOneActiveAlertPerWard(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	_ = s.UpsertWard(ctx, ward("w1"))
	a := domain.Alert{WardID: "w1", Level: domain.LevelWatch, PeakLevel: domain.LevelWatch, RuleVersion: "v1", RuleSHA256: "x",
		TriggeredAt: t0, Signals: []byte(`{}`), ReasonText: "r"}
	if err := s.CreateAlert(ctx, &a); err != nil {
		t.Fatal(err)
	}
	b := a
	if err := s.CreateAlert(ctx, &b); err == nil {
		t.Fatal("second active alert for same ward was accepted")
	}
}

func TestTxRollback(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	sentinel := errors.New("boom")
	err := s.Tx(ctx, func(tx *store.Store) error {
		if err := tx.UpsertWard(ctx, ward("rollback")); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.GetWard(ctx, "rollback"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("rolled-back ward persisted")
	}
}
