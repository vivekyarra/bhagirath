// Package backtest replays documented outbreak timelines through the frozen
// alert rule — the same rule.Evaluate that runs in production — to measure
// whether Bhagirath would have warned before people died (spec Section 9).
//
// Honesty rules implemented here:
//   - Only rows marked in_ward=yes are used.
//   - Rows are placed in time mechanically from their date ranges under two
//     scenarios: "best" (spread from the earliest plausible date) and
//     "conservative" (everything at the latest plausible date).
//   - No complaint has a geocoded point, so the 300 m cluster can never fire;
//     only the "open > 7 days without verified fix" condition can.
//   - Illness rows count max(diarrhoea, vomiting) as the frozen rule does;
//     jaundice/typhoid rows without those counts are not counted.
package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/rule"
)

// Row is one timeline event from a <slug>.csv file.
type Row struct {
	ID           string
	Kind         string
	InWard       bool
	Earliest     time.Time // zero when the source gives no lower bound
	Latest       time.Time
	Count        int // 0 when not stated
	IssueType    domain.IssueType
	Parameter    string
	Contaminated *bool
	Diarrhoea    int
	Vomiting     int
	SourceURL    string
}

// Outbreak is one entry of outbreaks.yaml.
type Outbreak struct {
	Slug                   string `yaml:"slug"`
	Name                   string `yaml:"name"`
	Period                 string `yaml:"period"`
	FirstHospitalisation   string `yaml:"first_hospitalisation"`
	FirstHospOfficial      string `yaml:"first_hospitalisation_official"`
	FirstDeath             string `yaml:"first_death"`
	FirstDeathCorroborated string `yaml:"first_death_corroborated"`
	DeathsReported         string `yaml:"deaths_reported"`
	DataQuality            string `yaml:"data_quality"`
	Rows                   []Row  `yaml:"-"`
}

// Variant is a pre-declared sensitivity analysis (sensitivity.yaml).
type Variant struct {
	Slug           string   `yaml:"slug"`
	Name           string   `yaml:"name"`
	Why            string   `yaml:"why"`
	Exclude        []string `yaml:"exclude"`
	OnlyComplaints []string `yaml:"only_complaints"`
}

// Scenario places rows in time.
type Scenario string

// Scenarios.
const (
	Best         Scenario = "best"
	Conservative Scenario = "conservative"
)

func date(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return time.Time{}, nil
	}
	d, err := time.ParseInLocation("2006-01-02", s, clock.IST)
	if err != nil {
		return time.Time{}, err
	}
	return d.Add(12 * time.Hour).UTC(), nil // midday IST
}

// LoadDir reads outbreaks.yaml, every <slug>.csv in dataDir, and sensitivity.yaml.
func LoadDir(dir string) ([]Outbreak, []Variant, error) {
	b, err := os.ReadFile(filepath.Join(dir, "outbreaks.yaml"))
	if err != nil {
		return nil, nil, err
	}
	var obs []Outbreak
	if err := yaml.Unmarshal(b, &obs); err != nil {
		return nil, nil, fmt.Errorf("outbreaks.yaml: %w", err)
	}
	for i := range obs {
		f, err := os.Open(filepath.Join(dir, "data", obs[i].Slug+".csv"))
		if err != nil {
			return nil, nil, err
		}
		rows, err := ReadCSV(f)
		f.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", obs[i].Slug, err)
		}
		obs[i].Rows = rows
	}
	var vs []Variant
	if b, err := os.ReadFile(filepath.Join(dir, "sensitivity.yaml")); err == nil {
		if err := yaml.Unmarshal(b, &vs); err != nil {
			return nil, nil, fmt.Errorf("sensitivity.yaml: %w", err)
		}
	}
	return obs, vs, nil
}

// ReadCSV parses a timeline CSV.
func ReadCSV(r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 1 {
		return nil, fmt.Errorf("empty file")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[h] = i
	}
	for _, need := range []string{"event_id", "kind", "in_ward", "date_earliest", "date_latest", "count", "issue_type", "parameter", "contaminated", "diarrhoea", "vomiting", "source_url"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("missing column %s", need)
		}
	}
	atoi := func(s string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	var out []Row
	for i, rec := range recs[1:] {
		g := func(k string) string { return rec[col[k]] }
		row := Row{ID: g("event_id"), Kind: g("kind"), InWard: g("in_ward") == "yes", Count: atoi(g("count")), IssueType: domain.IssueType(g("issue_type")),
			Parameter: strings.TrimSpace(g("parameter")), Diarrhoea: atoi(g("diarrhoea")), Vomiting: atoi(g("vomiting")), SourceURL: g("source_url")}
		if row.Earliest, err = date(g("date_earliest")); err != nil {
			return nil, fmt.Errorf("row %d: %w", i+2, err)
		}
		if row.Latest, err = date(g("date_latest")); err != nil {
			return nil, fmt.Errorf("row %d: %w", i+2, err)
		}
		if row.Latest.IsZero() {
			return nil, fmt.Errorf("row %d (%s): date_latest is required", i+2, row.ID)
		}
		if row.Earliest.IsZero() {
			row.Earliest = row.Latest // undated: known only by the time it was reported
		}
		if row.Earliest.After(row.Latest) {
			return nil, fmt.Errorf("row %s: earliest after latest", row.ID)
		}
		switch strings.ToLower(strings.TrimSpace(g("contaminated"))) {
		case "true":
			t := true
			row.Contaminated = &t
		case "false":
			f := false
			row.Contaminated = &f
		}
		out = append(out, row)
	}
	return out, nil
}

// spread returns n instants: evenly from a to b (best) or all at b.
func spread(a, b time.Time, n int, s Scenario) []time.Time {
	out := make([]time.Time, n)
	for i := range out {
		switch {
		case s == Conservative:
			out[i] = b
		case n == 1:
			out[i] = a
		default:
			out[i] = a.Add(time.Duration(float64(b.Sub(a)) * float64(i) / float64(n-1)))
		}
	}
	return out
}

// Snapshot converts rows to rule signals for one scenario.
func Snapshot(o Outbreak, rows []Row, s Scenario) rule.Snapshot {
	snap := rule.Snapshot{WardID: o.Slug, WardName: o.Name}
	for _, r := range rows {
		if !r.InWard {
			continue
		}
		switch r.Kind {
		case "complaint":
			n := max(1, r.Count)
			it := r.IssueType
			if it == "" {
				it = domain.IssueDirtyWater
			}
			for i, t := range spread(r.Earliest, r.Latest, n, s) {
				snap.Complaints = append(snap.Complaints, rule.ComplaintSignal{ID: fmt.Sprintf("%s#%d", r.ID, i+1), IssueType: it, At: t})
			}
		case "test":
			if r.Contaminated == nil {
				continue
			}
			at := r.Latest
			if s == Best {
				at = r.Earliest
			}
			p := r.Parameter
			if p == "" || strings.EqualFold(p, "not stated") || strings.EqualFold(p, "not disclosed") {
				p = "contamination"
			}
			snap.Tests = append(snap.Tests, rule.TestSignal{ID: r.ID, SampledAt: at, Parameter: p, Contaminated: *r.Contaminated})
		case "illness":
			cases := max(r.Diarrhoea, r.Vomiting)
			if cases == 0 {
				continue // jaundice/typhoid/unspecified: not an input of the frozen rule
			}
			days := int(r.Latest.Sub(r.Earliest).Hours()/24) + 1
			if s == Conservative {
				days = 1
			}
			for d := 0; d < days; d++ {
				c := cases / days
				if d < cases%days {
					c++
				}
				if c == 0 {
					continue
				}
				at := r.Latest
				if s == Best {
					at = r.Earliest.Add(time.Duration(d) * 24 * time.Hour)
				}
				snap.Illness = append(snap.Illness, rule.IllnessSignal{ID: fmt.Sprintf("%s#%d", r.ID, d+1), At: at, Diarrhoea: c})
			}
		}
	}
	return snap
}

// Result is the replay outcome for one outbreak × scenario × variant.
type Result struct {
	Slug, Name, Variant string
	Scenario            Scenario
	RuleVersion         string
	RuleSHA256          string
	FirstAt             map[domain.Level]time.Time // first time each level (or higher) was reached
	ReasonAt            map[domain.Level]string
	FirstSignal         map[string]time.Time // C, T, I
	CommissionerBy      time.Time            // rung 3 receives an evidence pack (no acknowledgements assumed)
	HealthOfficerBy     time.Time            // rung H receives an evidence pack
	Inputs              struct{ Complaints, Tests, Illness int }
}

// Replay evaluates the rule every 6 hours and at every event.
func Replay(r *rule.Rule, o Outbreak, rows []Row, s Scenario, ladderHours map[domain.Level]int) Result {
	snap := Snapshot(o, rows, s)
	res := Result{Slug: o.Slug, Name: o.Name, Scenario: s, RuleVersion: r.Version, RuleSHA256: r.SHA256,
		FirstAt: map[domain.Level]time.Time{}, ReasonAt: map[domain.Level]string{}, FirstSignal: map[string]time.Time{}}
	res.Inputs.Complaints, res.Inputs.Tests, res.Inputs.Illness = len(snap.Complaints), len(snap.Tests), len(snap.Illness)
	var times []time.Time
	var lo, hi time.Time
	add := func(t time.Time) {
		times = append(times, t, t.Add(7*24*time.Hour+time.Minute))
		if lo.IsZero() || t.Before(lo) {
			lo = t
		}
		if t.After(hi) {
			hi = t
		}
	}
	for _, c := range snap.Complaints {
		add(c.At)
	}
	for _, t := range snap.Tests {
		add(t.SampledAt)
	}
	for _, i := range snap.Illness {
		add(i.At)
	}
	if lo.IsZero() {
		return res
	}
	for t := lo.Truncate(6 * time.Hour); !t.After(hi.Add(8 * 24 * time.Hour)); t = t.Add(6 * time.Hour) {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	for _, t := range times {
		ev := r.Evaluate(snap, t)
		for l := domain.LevelWatch; l <= ev.Level; l++ {
			if _, ok := res.FirstAt[l]; !ok {
				res.FirstAt[l] = t
				res.ReasonAt[l] = ev.Reason
			}
		}
		for name, fired := range map[string]bool{"C": ev.C.Fired, "T": ev.T.Fired, "I": ev.I.Fired} {
			if _, ok := res.FirstSignal[name]; fired && !ok {
				res.FirstSignal[name] = t
			}
		}
	}
	res.CommissionerBy, res.HealthOfficerBy = ladder(res.FirstAt, ladderHours)
	return res
}

// ladder computes when rung 3 (commissioner) and rung H (health officer)
// would have received an evidence pack if nobody acknowledged anything,
// following the escalation engine: Watch→rung 1, timeout→2, timeout→3;
// Warning→1,2,H with 48 h deadlines (timeout of 2/H→3); Critical→all rungs.
func ladder(first map[domain.Level]time.Time, h map[domain.Level]int) (time.Time, time.Time) {
	var l3, hh time.Time
	min := func(cur, t time.Time) time.Time {
		if cur.IsZero() || t.Before(cur) {
			return t
		}
		return cur
	}
	if t, ok := first[domain.LevelWatch]; ok {
		d := time.Duration(h[domain.LevelWatch]) * time.Hour
		l3 = min(l3, t.Add(2*d))
	}
	if t, ok := first[domain.LevelWarning]; ok {
		l3 = min(l3, t.Add(time.Duration(h[domain.LevelWarning])*time.Hour))
		hh = min(hh, t)
	}
	if t, ok := first[domain.LevelCritical]; ok {
		l3, hh = min(l3, t), min(hh, t)
	}
	return l3, hh
}

// Filter applies a variant to rows.
func Filter(rows []Row, v Variant) []Row {
	ex := map[string]bool{}
	for _, id := range v.Exclude {
		ex[id] = true
	}
	only := map[string]bool{}
	for _, id := range v.OnlyComplaints {
		only[id] = true
	}
	var out []Row
	for _, r := range rows {
		if ex[r.ID] {
			continue
		}
		if len(only) > 0 && r.Kind == "complaint" && !only[r.ID] {
			continue
		}
		out = append(out, r)
	}
	return out
}
