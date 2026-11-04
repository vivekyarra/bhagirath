package geo

import (
	"math"
	"testing"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

func TestDistanceKnownValues(t *testing.T) {
	// Indore (Rajwada) to Bhopal (approx. 170 km great-circle).
	d := DistanceM(domain.Point{Lat: 22.7186, Lon: 75.8550}, domain.Point{Lat: 23.2599, Lon: 77.4126})
	if math.Abs(d-169_000) > 10_000 {
		t.Fatalf("Indore-Bhopal = %.0f m", d)
	}
	// 0.0027 degrees of latitude is ~300 m.
	d = DistanceM(domain.Point{Lat: 22.72, Lon: 75.86}, domain.Point{Lat: 22.7227, Lon: 75.86})
	if math.Abs(d-300) > 2 {
		t.Fatalf("300 m check = %.2f", d)
	}
	if DistanceM(domain.Point{Lat: 1, Lon: 1}, domain.Point{Lat: 1, Lon: 1}) != 0 {
		t.Fatal("zero distance")
	}
}

func TestPointInPolygon(t *testing.T) {
	sq := []domain.Point{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 10}, {Lat: 10, Lon: 10}, {Lat: 10, Lon: 0}}
	hole := []domain.Point{{Lat: 4, Lon: 4}, {Lat: 4, Lon: 6}, {Lat: 6, Lon: 6}, {Lat: 6, Lon: 4}}
	poly := [][]domain.Point{sq, hole}
	cases := []struct {
		p    domain.Point
		want bool
	}{
		{domain.Point{Lat: 1, Lon: 1}, true},
		{domain.Point{Lat: 5, Lon: 5}, false}, // in hole
		{domain.Point{Lat: 11, Lon: 5}, false},
		{domain.Point{Lat: 0, Lon: 5}, true}, // on edge
		{domain.Point{Lat: 10, Lon: 10}, true},
	}
	for _, c := range cases {
		if got := InPolygon(c.p, poly); got != c.want {
			t.Errorf("InPolygon(%v) = %v", c.p, got)
		}
	}
	// Concave "L" shape.
	l := [][]domain.Point{{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 4}, {Lat: 2, Lon: 4}, {Lat: 2, Lon: 2}, {Lat: 4, Lon: 2}, {Lat: 4, Lon: 0}}}
	if InPolygon(domain.Point{Lat: 3, Lon: 3}, l) || !InPolygon(domain.Point{Lat: 3, Lon: 1}, l) {
		t.Error("concave polygon wrong")
	}
}

func TestWardOf(t *testing.T) {
	wards := []domain.Ward{
		{ID: "a", Polygon: [][]domain.Point{{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}}, Centroid: domain.Point{Lat: 0.5, Lon: 0.5}},
		{ID: "b", Polygon: [][]domain.Point{{{Lat: 2, Lon: 2}, {Lat: 2, Lon: 3}, {Lat: 3, Lon: 3}, {Lat: 3, Lon: 2}}}, Centroid: domain.Point{Lat: 2.5, Lon: 2.5}},
	}
	if WardOf(domain.Point{Lat: 2.5, Lon: 2.2}, wards) != "b" || WardOf(domain.Point{Lat: 5, Lon: 5}, wards) != "" {
		t.Fatal("WardOf wrong")
	}
	if id, _ := NearestWard(domain.Point{Lat: 1.2, Lon: 1.2}, wards); id != "a" {
		t.Fatal("NearestWard wrong")
	}
}
