package locate

import (
	"testing"

	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
)

func wards(t *testing.T) []domain.Ward {
	l, err := config.LoadLadder("../../config/ladder.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.Ward
	for _, w := range l.Wards {
		out = append(out, w.ToWard())
	}
	return out
}

func TestPrecedence(t *testing.T) {
	ws := wards(t)
	inside := &domain.Point{Lat: 22.7200, Lon: 75.8600}
	r := Resolve(Input{Point: inside, Text: "panchayat bhawan ke paas"}, ws)
	if r.Method != domain.LocGPS || r.WardID != "demo-urban-12" {
		t.Fatalf("gps: %+v", r)
	}
	r = Resolve(Input{Text: "शिव मंदिर के पास गंदा पानी"}, ws)
	if r.Method != domain.LocLandmark || r.WardID != "demo-urban-12" || r.Landmark != "Shiv Mandir" || r.Point == nil {
		t.Fatalf("landmark hi: %+v", r)
	}
	r = Resolve(Input{Text: "near the overhead water tank there is a smell"}, ws)
	if r.Landmark != "Overhead Water Tank" {
		t.Fatalf("landmark en: %+v", r)
	}
	r = Resolve(Input{Text: "nothing useful", ReporterHome: &domain.Point{Lat: 22.6, Lon: 75.70}}, ws)
	if r.Method != domain.LocRegistered || r.WardID != "demo-village-3" {
		t.Fatalf("registered: %+v", r)
	}
	r = Resolve(Input{Text: "nothing useful", WardHint: "demo-village-3"}, ws)
	if r.Method != domain.LocWardOnly || !r.NeedsFollowup() {
		t.Fatalf("ward only: %+v", r)
	}
	r = Resolve(Input{Text: "nothing useful", WardHint: "no-such-ward"}, ws)
	if r.Resolved() {
		t.Fatalf("unknown ward hint accepted: %+v", r)
	}
	r = Resolve(Input{Point: &domain.Point{Lat: 28.6, Lon: 77.2}}, ws)
	if r.Resolved() || r.Point == nil {
		t.Fatalf("outside pin: %+v", r)
	}
}

func TestAliasWordBoundary(t *testing.T) {
	ws := wards(t)
	// "school" is an alias; "preschooler" must not match it.
	if r := Resolve(Input{Text: "my preschooler is sick"}, ws); r.Method == domain.LocLandmark {
		t.Fatalf("substring matched: %+v", r)
	}
}

func TestAmbiguousLandmarkUsesHint(t *testing.T) {
	ws := wards(t)
	dup := ws[1]
	dup.Landmarks = append(dup.Landmarks, domain.Landmark{Name: "Shiv Mandir", Point: domain.Point{Lat: 22.6, Lon: 75.70}})
	ws[1] = dup
	r := Resolve(Input{Text: "shiv mandir"}, ws)
	if r.Resolved() || len(r.Candidates) != 2 {
		t.Fatalf("ambiguous should not resolve: %+v", r)
	}
	r = Resolve(Input{Text: "shiv mandir", ReporterWard: "demo-village-3"}, ws)
	if r.WardID != "demo-village-3" || r.Method != domain.LocLandmark {
		t.Fatalf("hint not used: %+v", r)
	}
}
