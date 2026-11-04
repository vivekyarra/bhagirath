package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

// Ladder is the pilot definition: wards (with boundaries and landmarks) and
// the officials responsible for them.
type Ladder struct {
	Wards     []WardDef     `yaml:"wards"`
	Officials []OfficialDef `yaml:"officials"`
	// Escalation deadlines per alert level, in hours (Section 8.2).
	DeadlinesHours map[string]int `yaml:"deadlines_hours"`
	// Deadline for a single-complaint escalation, in hours.
	ComplaintDeadlineHours int `yaml:"complaint_deadline_hours"`
}

// WardDef is a ward in YAML. Polygon vertices are [lat, lon] pairs.
type WardDef struct {
	ID              string        `yaml:"id"`
	Name            string        `yaml:"name"`
	CityOrBlock     string        `yaml:"city_or_block"`
	District        string        `yaml:"district"`
	State           string        `yaml:"state"`
	DefaultLanguage string        `yaml:"default_language"`
	PartnerName     string        `yaml:"partner_name"`
	Demo            bool          `yaml:"demo"`
	Polygon         [][2]float64  `yaml:"polygon"`
	Landmarks       []LandmarkDef `yaml:"landmarks"`
}

// LandmarkDef is a named place in YAML.
type LandmarkDef struct {
	Name    string     `yaml:"name"`
	Aliases []string   `yaml:"aliases"`
	At      [2]float64 `yaml:"at"`
}

// OfficialDef is an official in YAML. Phone numbers belong only in the
// untracked local ladder file, never in the repository.
type OfficialDef struct {
	Name             string   `yaml:"name"`
	Role             string   `yaml:"role"`
	Department       string   `yaml:"department"`
	Wards            []string `yaml:"wards"`
	Level            string   `yaml:"level"`
	Email            string   `yaml:"email"`
	Phone            string   `yaml:"phone"`
	GrievanceChannel string   `yaml:"grievance_channel"`
	PreferredChannel string   `yaml:"preferred_channel"`
	Language         string   `yaml:"language"`
}

// LoadLadder parses and validates a ladder file.
func LoadLadder(path string) (*Ladder, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l Ladder
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &l, l.Validate()
}

// Validate checks referential integrity and geometry.
func (l *Ladder) Validate() error {
	ids := map[string]bool{}
	for _, w := range l.Wards {
		if w.ID == "" || w.Name == "" {
			return fmt.Errorf("ward needs id and name")
		}
		if ids[w.ID] {
			return fmt.Errorf("duplicate ward id %q", w.ID)
		}
		ids[w.ID] = true
		if len(w.Polygon) < 3 {
			return fmt.Errorf("ward %s: polygon needs at least 3 vertices", w.ID)
		}
		for _, v := range w.Polygon {
			if !(domain.Point{Lat: v[0], Lon: v[1]}).Valid() {
				return fmt.Errorf("ward %s: invalid vertex %v", w.ID, v)
			}
		}
	}
	for _, o := range l.Officials {
		if !domain.Contains(domain.LadderLevels, domain.LadderLevel(o.Level)) {
			return fmt.Errorf("official %q: level must be one of 1,2,3,H,P", o.Name)
		}
		if len(o.Wards) == 0 {
			return fmt.Errorf("official %q: no wards", o.Name)
		}
		for _, w := range o.Wards {
			if !ids[w] {
				return fmt.Errorf("official %q: unknown ward %q", o.Name, w)
			}
		}
		if o.Email == "" && o.Phone == "" && o.GrievanceChannel == "" {
			return fmt.Errorf("official %q: needs email, phone or grievance_channel", o.Name)
		}
		switch o.PreferredChannel {
		case "", "email", "whatsapp", "grievance":
		default:
			return fmt.Errorf("official %q: preferred_channel must be email, whatsapp or grievance", o.Name)
		}
	}
	for k, h := range l.DeadlinesHours {
		if _, err := domain.ParseLevel(k); err != nil || h <= 0 {
			return fmt.Errorf("deadlines_hours: bad entry %s=%d", k, h)
		}
	}
	return nil
}

// Deadline returns the escalation deadline (hours) for an alert level,
// defaulting to the spec (Watch 72h, Warning 48h, Critical 24h).
func (l *Ladder) Deadline(level domain.Level) int {
	if h, ok := l.DeadlinesHours[level.String()]; ok {
		return h
	}
	switch level {
	case domain.LevelCritical:
		return 24
	case domain.LevelWarning:
		return 48
	default:
		return 72
	}
}

// ComplaintDeadline is the deadline for a single-complaint escalation.
func (l *Ladder) ComplaintDeadline() int {
	if l.ComplaintDeadlineHours > 0 {
		return l.ComplaintDeadlineHours
	}
	return 72
}

// ToWard converts a definition to a domain ward.
func (w WardDef) ToWard() domain.Ward {
	ring := make([]domain.Point, len(w.Polygon))
	var sl, so float64
	for i, v := range w.Polygon {
		ring[i] = domain.Point{Lat: v[0], Lon: v[1]}
		sl += v[0]
		so += v[1]
	}
	lms := make([]domain.Landmark, len(w.Landmarks))
	for i, lm := range w.Landmarks {
		lms[i] = domain.Landmark{Name: lm.Name, Aliases: lm.Aliases, Point: domain.Point{Lat: lm.At[0], Lon: lm.At[1]}}
	}
	lang := w.DefaultLanguage
	if lang == "" {
		lang = "hi"
	}
	return domain.Ward{ID: w.ID, Name: w.Name, CityOrBlock: w.CityOrBlock, District: w.District, State: w.State,
		Polygon: [][]domain.Point{ring}, Centroid: domain.Point{Lat: sl / float64(len(ring)), Lon: so / float64(len(ring))},
		DefaultLanguage: lang, Landmarks: lms, PartnerName: w.PartnerName, Demo: w.Demo}
}
