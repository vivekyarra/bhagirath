// Package geo implements the small amount of spherical geometry Bhagirath
// needs: distances, point-in-polygon and ward resolution.
package geo

import (
	"math"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const earthRadiusM = 6371008.8

// DistanceM returns the great-circle (haversine) distance in metres.
func DistanceM(a, b domain.Point) float64 {
	la1, la2 := rad(a.Lat), rad(b.Lat)
	dla := la2 - la1
	dlo := rad(b.Lon - a.Lon)
	h := math.Sin(dla/2)*math.Sin(dla/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dlo/2)*math.Sin(dlo/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(h)))
}

func rad(d float64) float64 { return d * math.Pi / 180 }

// InRing reports whether p lies inside a closed ring (even-odd rule). Points
// exactly on an edge count as inside.
func InRing(p domain.Point, ring []domain.Point) bool {
	n := len(ring)
	if n < 3 {
		return false
	}
	inside := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if onSegment(p, a, b) {
			return true
		}
		if (a.Lat > p.Lat) != (b.Lat > p.Lat) {
			x := (b.Lon-a.Lon)*(p.Lat-a.Lat)/(b.Lat-a.Lat) + a.Lon
			if p.Lon < x {
				inside = !inside
			}
		}
	}
	return inside
}

func onSegment(p, a, b domain.Point) bool {
	cross := (p.Lat-a.Lat)*(b.Lon-a.Lon) - (p.Lon-a.Lon)*(b.Lat-a.Lat)
	if math.Abs(cross) > 1e-12 {
		return false
	}
	return p.Lat >= math.Min(a.Lat, b.Lat)-1e-12 && p.Lat <= math.Max(a.Lat, b.Lat)+1e-12 &&
		p.Lon >= math.Min(a.Lon, b.Lon)-1e-12 && p.Lon <= math.Max(a.Lon, b.Lon)+1e-12
}

// InPolygon reports whether p is inside the outer ring and outside every hole.
func InPolygon(p domain.Point, poly [][]domain.Point) bool {
	if len(poly) == 0 || !InRing(p, poly[0]) {
		return false
	}
	for _, hole := range poly[1:] {
		if InRing(p, hole) {
			return false
		}
	}
	return true
}

// WardOf returns the id of the ward containing p, or "".
func WardOf(p domain.Point, wards []domain.Ward) string {
	for _, w := range wards {
		if InPolygon(p, w.Polygon) {
			return w.ID
		}
	}
	return ""
}

// NearestWard returns the ward whose boundary-centroid is nearest to p and
// the distance, for "just outside the pilot area" diagnostics.
func NearestWard(p domain.Point, wards []domain.Ward) (string, float64) {
	best, bestD := "", math.Inf(1)
	for _, w := range wards {
		if d := DistanceM(p, w.Centroid); d < bestD {
			best, bestD = w.ID, d
		}
	}
	return best, bestD
}
