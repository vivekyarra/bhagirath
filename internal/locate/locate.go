// Package locate resolves a complaint to a ward and, when possible, a point:
// GPS pin > named landmark in the transcript > the reporter's registered
// address > the channel's ward (e.g. a ward-specific number) > unresolved.
package locate

import (
	"strings"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/geo"
	"github.com/vivekyarra/bhagirath/internal/lang"
)

// Input is everything known about where a complaint came from.
type Input struct {
	Point        *domain.Point
	Text         string // transcript and/or typed landmark
	ReporterHome *domain.Point
	ReporterWard string
	WardHint     string // ward chosen on the web form, IVR number, or by a volunteer
}

// Result is a resolved location.
type Result struct {
	WardID     string                `json:"ward_id"`
	Point      *domain.Point         `json:"point,omitempty"`
	Method     domain.LocationMethod `json:"method"`
	Landmark   string                `json:"landmark,omitempty"`
	Candidates []string              `json:"candidates,omitempty"` // ambiguous landmark matches
	Note       string                `json:"note,omitempty"`
}

// Resolved reports whether a ward was found.
func (r Result) Resolved() bool { return r.WardID != "" }

// NeedsFollowup reports whether Bhagirath should ask the reporter for a
// landmark or pin (no ward, or ward known but no point to cluster with).
func (r Result) NeedsFollowup() bool { return r.WardID == "" || r.Point == nil }

type match struct {
	ward     string
	landmark domain.Landmark
	strength int
}

// Resolve applies the precedence rules.
func Resolve(in Input, wards []domain.Ward) Result {
	if in.Point != nil && in.Point.Valid() {
		if w := geo.WardOf(*in.Point, wards); w != "" {
			p := *in.Point
			return Result{WardID: w, Point: &p, Method: domain.LocGPS}
		}
		near, d := geo.NearestWard(*in.Point, wards)
		_ = near
		if d > 0 {
			// A pin outside every pilot ward is recorded but not forced into a ward.
			p := *in.Point
			return Result{Point: &p, Method: domain.LocUnresolved, Note: "pin is outside the pilot wards"}
		}
	}
	if m := landmarks(in.Text, wards); len(m) > 0 {
		pick := pickMatch(m, in)
		if pick != nil {
			p := pick.landmark.Point
			return Result{WardID: pick.ward, Point: &p, Method: domain.LocLandmark, Landmark: pick.landmark.Name}
		}
		var c []string
		for _, x := range m {
			c = append(c, x.landmark.Name+" ("+x.ward+")")
		}
		res := fallback(in, wards)
		res.Candidates = c
		return res
	}
	return fallback(in, wards)
}

func fallback(in Input, wards []domain.Ward) Result {
	if in.ReporterHome != nil && in.ReporterHome.Valid() {
		if w := geo.WardOf(*in.ReporterHome, wards); w != "" {
			p := *in.ReporterHome
			return Result{WardID: w, Point: &p, Method: domain.LocRegistered}
		}
	}
	for _, w := range []string{in.WardHint, in.ReporterWard} {
		if w != "" && known(w, wards) {
			return Result{WardID: w, Method: domain.LocWardOnly}
		}
	}
	return Result{Method: domain.LocUnresolved}
}

func known(id string, wards []domain.Ward) bool {
	for _, w := range wards {
		if w.ID == id {
			return true
		}
	}
	return false
}

// pickMatch chooses the strongest landmark, preferring the hinted ward; it
// returns nil when equally strong matches point to different wards.
func pickMatch(ms []match, in Input) *match {
	best := ms[0]
	for _, m := range ms[1:] {
		if m.strength > best.strength {
			best = m
		}
	}
	var top []match
	for _, m := range ms {
		if m.strength == best.strength {
			top = append(top, m)
		}
	}
	wardsSeen := map[string]bool{}
	for _, m := range top {
		wardsSeen[m.ward] = true
	}
	if len(wardsSeen) == 1 {
		return &top[0]
	}
	for _, hint := range []string{in.WardHint, in.ReporterWard} {
		for i := range top {
			if hint != "" && top[i].ward == hint {
				return &top[i]
			}
		}
	}
	return nil
}

func landmarks(text string, wards []domain.Ward) []match {
	norm := " " + lang.Normalize(text) + " "
	if strings.TrimSpace(norm) == "" {
		return nil
	}
	var out []match
	for _, w := range wards {
		for _, lm := range w.Landmarks {
			best := 0
			for _, name := range append([]string{lm.Name}, lm.Aliases...) {
				n := lang.Normalize(name)
				if n == "" {
					continue
				}
				if strings.Contains(norm, " "+n+" ") || (!isASCII(n) && strings.Contains(norm, n)) {
					if l := len([]rune(n)); l > best {
						best = l
					}
				}
			}
			if best > 0 {
				out = append(out, match{ward: w.ID, landmark: lm, strength: best})
			}
		}
	}
	return out
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
