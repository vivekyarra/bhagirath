package config

import (
	"testing"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

func TestExampleLadderIsValid(t *testing.T) {
	l, err := LoadLadder("../../config/ladder.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Wards) < 2 {
		t.Fatalf("want 2+ demo wards, got %d", len(l.Wards))
	}
	for _, w := range l.Wards {
		if !w.Demo {
			t.Errorf("example ward %s must be marked demo", w.ID)
		}
		dw := w.ToWard()
		if !dw.Centroid.Valid() || len(dw.Polygon[0]) != len(w.Polygon) {
			t.Errorf("ward %s conversion broken", w.ID)
		}
	}
	for _, o := range l.Officials {
		if o.Phone != "" {
			t.Errorf("official %q has a phone number in the committed example file", o.Name)
		}
	}
	if l.Deadline(domain.LevelWatch) != 72 || l.Deadline(domain.LevelWarning) != 48 || l.Deadline(domain.LevelCritical) != 24 {
		t.Error("deadlines differ from spec Section 8.2")
	}
}

func TestLadderValidationRejectsBadInput(t *testing.T) {
	cases := []Ladder{
		{Wards: []WardDef{{ID: "a", Name: "A", Polygon: [][2]float64{{1, 1}, {1, 2}}}}},
		{Wards: []WardDef{{ID: "a", Name: "A", Polygon: [][2]float64{{1, 1}, {1, 2}, {2, 2}}}},
			Officials: []OfficialDef{{Name: "x", Level: "9", Wards: []string{"a"}, Email: "e"}}},
		{Wards: []WardDef{{ID: "a", Name: "A", Polygon: [][2]float64{{1, 1}, {1, 2}, {2, 2}}}},
			Officials: []OfficialDef{{Name: "x", Level: "1", Wards: []string{"zzz"}, Email: "e"}}},
		{Wards: []WardDef{{ID: "a", Name: "A", Polygon: [][2]float64{{1, 1}, {1, 2}, {2, 2}}}},
			Officials: []OfficialDef{{Name: "x", Level: "1", Wards: []string{"a"}}}},
	}
	for i, l := range cases {
		if err := l.Validate(); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestPilotModeRequiresSecrets(t *testing.T) {
	t.Setenv("BHAGIRATH_MODE", "pilot")
	t.Setenv("BHAGIRATH_SECRET_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("pilot mode started without secret key")
	}
	t.Setenv("BHAGIRATH_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("BHAGIRATH_ADMIN_PASSWORD", "short")
	if _, err := Load(); err == nil {
		t.Fatal("pilot mode accepted weak admin password")
	}
	t.Setenv("BHAGIRATH_MODE", "demo")
	if _, err := Load(); err != nil {
		t.Fatalf("demo mode: %v", err)
	}
}
