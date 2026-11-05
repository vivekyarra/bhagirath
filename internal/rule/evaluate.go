package rule

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/geo"
)

// ComplaintSignal is a complaint as the rule sees it.
type ComplaintSignal struct {
	ID              string           `json:"id"`
	Reporter        string           `json:"-"` // stable reporter key; "" = anonymous
	IssueType       domain.IssueType `json:"issue_type"`
	At              time.Time        `json:"at"`
	Point           *domain.Point    `json:"-"`
	Invalid         bool             `json:"-"`
	VerifiedFixedAt *time.Time       `json:"-"`
}

// TestSignal is one water-test parameter result.
type TestSignal struct {
	ID           string    `json:"id"`
	SampledAt    time.Time `json:"sampled_at"`
	Parameter    string    `json:"parameter"`
	Contaminated bool      `json:"contaminated"`
	Value        *float64  `json:"value,omitempty"`
	Limit        *float64  `json:"limit,omitempty"`
	Unit         string    `json:"unit,omitempty"`
}

// IllnessSignal is one aggregate illness / ORS report.
type IllnessSignal struct {
	ID        string    `json:"id"`
	Reporter  string    `json:"-"` // stable key of the clinic/chemist/ASHA
	At        time.Time `json:"at"`
	Diarrhoea int       `json:"diarrhoea"`
	Vomiting  int       `json:"vomiting"`
	ORS       *int      `json:"ors,omitempty"`
}

// Snapshot is everything known about one ward.
type Snapshot struct {
	WardID     string
	WardName   string
	Complaints []ComplaintSignal
	Tests      []TestSignal
	Illness    []IllnessSignal
}

// CResult explains signal C.
type CResult struct {
	Fired             bool             `json:"fired"`
	Cluster           bool             `json:"cluster"`
	ClusterIDs        []string         `json:"cluster_ids,omitempty"`
	ClusterHouseholds int              `json:"cluster_households,omitempty"`
	ClusterSpanM      float64          `json:"cluster_span_m,omitempty"`
	ClusterDays       int              `json:"cluster_days,omitempty"`
	ClusterIssue      domain.IssueType `json:"cluster_issue,omitempty"`
	Stale             bool             `json:"stale"`
	StaleIDs          []string         `json:"stale_ids,omitempty"`
	OldestStaleDays   int              `json:"oldest_stale_days,omitempty"`
}

// TResult explains signal T.
type TResult struct {
	Fired   bool         `json:"fired"`
	Failing []TestSignal `json:"failing,omitempty"`
}

// ORSSpike describes one chemist's ORS sales spike.
type ORSSpike struct {
	ReportID string    `json:"report_id"`
	At       time.Time `json:"at"`
	Units    int       `json:"units"`
	Baseline float64   `json:"baseline"`
	Ratio    float64   `json:"ratio"`
}

// IResult explains signal I.
type IResult struct {
	Fired     bool       `json:"fired"`
	Cases     int        `json:"cases"`
	CaseFired bool       `json:"case_fired"`
	ReportIDs []string   `json:"report_ids,omitempty"`
	ORSSpikes []ORSSpike `json:"ors_spikes,omitempty"`
}

// Result is the rule's verdict for one ward at one instant.
type Result struct {
	WardID      string       `json:"ward_id"`
	At          time.Time    `json:"at"`
	Level       domain.Level `json:"level"`
	C           CResult      `json:"c"`
	T           TResult      `json:"t"`
	I           IResult      `json:"i"`
	SignalCount int          `json:"signal_count"`
	Reason      string       `json:"reason"`
	ReasonHI    string       `json:"reason_hi"`
	RuleVersion string       `json:"rule_version"`
	RuleSHA256  string       `json:"rule_sha256"`
	// Coalesced is set (by callers, never by Evaluate) when an evaluation
	// request was folded into one already running for the same ward.
	Coalesced bool `json:"-"`
}

// Evaluate applies the rule to a snapshot at time now. It is a pure function:
// identical inputs always give identical outputs, and only events at or
// before now are considered (no look-ahead in backtests).
func (r *Rule) Evaluate(s Snapshot, now time.Time) Result {
	res := Result{WardID: s.WardID, At: now, RuleVersion: r.Version, RuleSHA256: r.SHA256}
	res.C = r.evalC(s.Complaints, now)
	res.T = r.evalT(s.Tests, now)
	res.I = r.evalI(s.Illness, now)
	for _, f := range []bool{res.C.Fired, res.T.Fired, res.I.Fired} {
		if f {
			res.SignalCount++
		}
	}
	switch {
	case res.SignalCount == 3,
		res.I.Fired && (res.C.Fired || res.T.Fired) && res.I.Cases >= r.Levels.CriticalMinIllnessCases:
		res.Level = domain.LevelCritical
	case res.SignalCount == 2:
		res.Level = domain.LevelWarning
	case res.SignalCount == 1:
		res.Level = domain.LevelWatch
	}
	if res.Level != domain.LevelNone {
		res.Reason = reasonEN(r, s.WardName, res)
		res.ReasonHI = reasonHI(r, s.WardName, res)
	}
	return res
}

func (r *Rule) countsAsC(c ComplaintSignal) bool {
	return !c.Invalid && domain.Contains(r.ComplaintCluster.IssueTypes, c.IssueType)
}

func fixedBy(c ComplaintSignal, now time.Time) bool {
	return c.VerifiedFixedAt != nil && !c.VerifiedFixedAt.After(now)
}

func (r *Rule) evalC(all []ComplaintSignal, now time.Time) CResult {
	cfg := r.ComplaintCluster
	var out CResult
	from := now.Add(-time.Duration(cfg.WindowDays) * 24 * time.Hour)
	staleBefore := now.Add(-time.Duration(cfg.StaleOpenDays) * 24 * time.Hour)

	var pts []ComplaintSignal
	for _, c := range all {
		if !r.countsAsC(c) || c.At.After(now) {
			continue
		}
		if c.At.Before(staleBefore) && !fixedBy(c, now) {
			out.Stale = true
			out.StaleIDs = append(out.StaleIDs, c.ID)
			if d := int(now.Sub(c.At).Hours() / 24); d > out.OldestStaleDays {
				out.OldestStaleDays = d
			}
		}
		if c.At.Before(from) || (cfg.ExcludeVerifiedFixed && fixedBy(c, now)) {
			continue
		}
		if c.Point == nil && !cfg.UnlocatedCountTowardCluster {
			continue
		}
		pts = append(pts, c)
	}
	sort.Slice(pts, func(i, j int) bool {
		if !pts[i].At.Equal(pts[j].At) {
			return pts[i].At.Before(pts[j].At)
		}
		return pts[i].ID < pts[j].ID
	})
	if best := r.bestCluster(pts); best != nil {
		out.Cluster = true
		out.ClusterHouseholds = r.distinctReporters(best)
		first, last := best[0].At, best[0].At
		issues := map[domain.IssueType]int{}
		for i, a := range best {
			out.ClusterIDs = append(out.ClusterIDs, a.ID)
			issues[a.IssueType]++
			if a.At.Before(first) {
				first = a.At
			}
			if a.At.After(last) {
				last = a.At
			}
			for _, b := range best[i+1:] {
				if a.Point != nil && b.Point != nil {
					out.ClusterSpanM = math.Max(out.ClusterSpanM, geo.DistanceM(*a.Point, *b.Point))
				}
			}
		}
		out.ClusterDays = max(1, int(math.Ceil(last.Sub(first).Hours()/24)))
		out.ClusterIssue = dominantIssue(issues)
	}
	out.Fired = out.Cluster || out.Stale
	return out
}

func (r *Rule) reporterKey(c ComplaintSignal) string {
	if c.Reporter == "" {
		if r.ComplaintCluster.AnonymousReportersAreOne {
			return "\x00anonymous"
		}
		return "\x00anon:" + c.ID
	}
	return c.Reporter
}

func (r *Rule) distinctReporters(cs []ComplaintSignal) int {
	seen := map[string]bool{}
	for _, c := range cs {
		seen[r.reporterKey(c)] = true
	}
	return len(seen)
}

// bestCluster returns a set of complaints that are pairwise within the
// radius and satisfy both minimums, or nil if none exists.
//
// Existence is decided exactly. With the v1 thresholds (3 complaints, 3
// households) a qualifying set exists iff there is a triangle of pairwise-close
// complaints from three distinct households, found in O(n·d²). For other
// thresholds the exact Bron–Kerbosch search over maximal cliques is used. The
// returned set (for the reason text) is then grown greedily from the valid
// seeds, so it reports at least as many households as the seed, never more
// than truly exist.
func (r *Rule) bestCluster(pts []ComplaintSignal) []ComplaintSignal {
	cfg := r.ComplaintCluster
	n := len(pts)
	if n < cfg.MinComplaints {
		return nil
	}
	adj := make([][]bool, n)
	nb := make([][]int, n)
	for i := range adj {
		adj[i] = make([]bool, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			ok := pts[i].Point == nil || pts[j].Point == nil || geo.DistanceM(*pts[i].Point, *pts[j].Point) <= cfg.PairwiseRadiusM
			if ok {
				adj[i][j], adj[j][i] = true, true
				nb[i] = append(nb[i], j)
				nb[j] = append(nb[j], i)
			}
		}
	}
	keys := make([]string, n)
	for i, p := range pts {
		keys[i] = r.reporterKey(p)
	}
	var seeds [][]int
	if cfg.MinComplaints <= 3 && cfg.MinDistinctReporters <= 3 {
		seeds = triangleSeeds(nb, adj, keys, cfg.MinDistinctReporters, 64)
	} else {
		seeds = r.cliqueSeeds(pts, adj)
	}
	var best []int
	bestR := 0
	for _, seed := range seeds {
		c := grow(seed, nb, adj, keys)
		dr := distinct(c, keys)
		if len(c) < cfg.MinComplaints || dr < cfg.MinDistinctReporters {
			continue
		}
		if dr > bestR || (dr == bestR && len(c) > len(best)) {
			best, bestR = c, dr
		}
	}
	if best == nil {
		return nil
	}
	sort.Ints(best)
	out := make([]ComplaintSignal, len(best))
	for i, k := range best {
		out[i] = pts[k]
	}
	return out
}

func distinct(idx []int, keys []string) int {
	seen := map[string]bool{}
	for _, i := range idx {
		seen[keys[i]] = true
	}
	return len(seen)
}

// triangleSeeds finds up to limit triangles (one per anchor) whose members
// come from at least minR distinct households.
func triangleSeeds(nb [][]int, adj [][]bool, keys []string, minR, limit int) [][]int {
	var out [][]int
	for i := range nb {
	anchor:
		for _, j := range nb[i] {
			if j <= i {
				continue
			}
			for _, k := range nb[i] {
				if k <= j || !adj[j][k] {
					continue
				}
				d := 1
				if keys[j] != keys[i] {
					d++
				}
				if keys[k] != keys[i] && keys[k] != keys[j] {
					d++
				}
				if d >= minR {
					out = append(out, []int{i, j, k})
					break anchor
				}
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

// grow extends a clique greedily, preferring complaints from new households.
func grow(seed []int, nb [][]int, adj [][]bool, keys []string) []int {
	c := append([]int(nil), seed...)
	in := map[int]bool{}
	have := map[string]bool{}
	for _, x := range c {
		in[x] = true
		have[keys[x]] = true
	}
	cands := append([]int(nil), nb[seed[0]]...)
	sort.SliceStable(cands, func(a, b int) bool {
		na, nb2 := !have[keys[cands[a]]], !have[keys[cands[b]]]
		if na != nb2 {
			return na
		}
		return cands[a] < cands[b]
	})
	for _, v := range cands {
		if in[v] {
			continue
		}
		ok := true
		for _, u := range c {
			if !adj[u][v] {
				ok = false
				break
			}
		}
		if ok {
			c = append(c, v)
			in[v] = true
			have[keys[v]] = true
		}
	}
	return c
}

// cliqueSeeds enumerates maximal cliques exactly (Bron–Kerbosch with pivot)
// for thresholds above three; bounded to keep evaluation predictable.
func (r *Rule) cliqueSeeds(pts []ComplaintSignal, adj [][]bool) [][]int {
	n := len(pts)
	var out [][]int
	var bk func(rset, p, x []int)
	bk = func(rset, p, x []int) {
		if len(out) >= 4096 {
			return
		}
		if len(p) == 0 && len(x) == 0 {
			if len(rset) >= r.ComplaintCluster.MinComplaints {
				out = append(out, append([]int(nil), rset...))
			}
			return
		}
		pivot, deg := -1, -1
		for _, u := range append(append([]int{}, p...), x...) {
			d := 0
			for _, v := range p {
				if adj[u][v] {
					d++
				}
			}
			if d > deg {
				pivot, deg = u, d
			}
		}
		cand := []int{}
		for _, v := range p {
			if pivot < 0 || !adj[pivot][v] {
				cand = append(cand, v)
			}
		}
		for _, v := range cand {
			np, nx := []int{}, []int{}
			for _, u := range p {
				if adj[v][u] {
					np = append(np, u)
				}
			}
			for _, u := range x {
				if adj[v][u] {
					nx = append(nx, u)
				}
			}
			bk(append(append([]int{}, rset...), v), np, nx)
			p = remove(p, v)
			x = append(x, v)
		}
	}
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	bk(nil, all, nil)
	return out
}

func remove(s []int, v int) []int {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func dominantIssue(m map[domain.IssueType]int) domain.IssueType {
	if len(m) > 1 {
		return "" // mixed
	}
	for k := range m {
		return k
	}
	return ""
}

func normParam(p string) string { return strings.ToLower(strings.TrimSpace(p)) }

func (r *Rule) evalT(tests []TestSignal, now time.Time) TResult {
	from := now.Add(-time.Duration(r.TestFailure.WindowDays) * 24 * time.Hour)
	var out TResult
	for _, t := range tests {
		if !t.Contaminated || t.SampledAt.After(now) || t.SampledAt.Before(from) {
			continue
		}
		cleared := false
		for _, u := range tests {
			if u.Contaminated || !u.SampledAt.After(t.SampledAt) || u.SampledAt.After(now) {
				continue
			}
			if r.TestFailure.RetestSameParameter && normParam(u.Parameter) != normParam(t.Parameter) {
				continue
			}
			cleared = true
			break
		}
		if !cleared {
			out.Failing = append(out.Failing, t)
		}
	}
	sort.Slice(out.Failing, func(i, j int) bool { return out.Failing[i].SampledAt.Before(out.Failing[j].SampledAt) })
	out.Fired = len(out.Failing) > 0
	return out
}

func (r *Rule) evalI(reports []IllnessSignal, now time.Time) IResult {
	cfg := r.IllnessCluster
	var out IResult
	from := now.Add(-time.Duration(cfg.WindowHours) * time.Hour)
	for _, rep := range reports {
		if rep.At.After(now) || !rep.At.After(from) {
			continue
		}
		cases := max(rep.Diarrhoea, rep.Vomiting)
		if cfg.CasesPerReport == "sum" {
			cases = rep.Diarrhoea + rep.Vomiting
		}
		if cases > 0 {
			out.Cases += cases
			out.ReportIDs = append(out.ReportIDs, rep.ID)
		}
	}
	out.CaseFired = out.Cases >= cfg.MinCases

	orsFrom := now.Add(-time.Duration(cfg.ORS.WindowHours) * time.Hour)
	for _, rep := range reports {
		if rep.ORS == nil || rep.Reporter == "" || rep.At.After(now) || !rep.At.After(orsFrom) {
			continue
		}
		baseFrom := rep.At.Add(-time.Duration(cfg.ORS.BaselineDays) * 24 * time.Hour)
		var sum float64
		var n int
		for _, b := range reports {
			if b.ID == rep.ID || b.Reporter != rep.Reporter || b.ORS == nil || !b.At.Before(rep.At) || b.At.Before(baseFrom) {
				continue
			}
			sum += float64(*b.ORS)
			n++
		}
		if n < cfg.ORS.MinBaselineReports || sum == 0 {
			continue // no meaningful average yet; never fire on a zero baseline
		}
		avg := sum / float64(n)
		if float64(*rep.ORS) >= cfg.ORS.Multiplier*avg {
			out.ORSSpikes = append(out.ORSSpikes, ORSSpike{ReportID: rep.ID, At: rep.At, Units: *rep.ORS, Baseline: round1(avg), Ratio: round1(float64(*rep.ORS) / avg)})
		}
	}
	out.Fired = out.CaseFired || len(out.ORSSpikes) > 0
	return out
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func day(t time.Time) string { return t.In(clock.IST).Format("2 Jan 2006") }

func issuePhraseEN(it domain.IssueType) string {
	switch it {
	case domain.IssueDirtyWater:
		return "dirty water"
	case domain.IssueFoulSmell:
		return "foul-smelling water"
	case domain.IssueSewageMixing:
		return "sewage mixing into drinking water"
	}
	return "dirty, foul-smelling or sewage-mixed water"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func reasonEN(r *Rule, ward string, res Result) string {
	var parts []string
	if res.C.Cluster {
		parts = append(parts, fmt.Sprintf("%s within %d m reported %s in %s",
			plural(res.C.ClusterHouseholds, "household", "households"), roundUp10(res.C.ClusterSpanM),
			issuePhraseEN(res.C.ClusterIssue), plural(res.C.ClusterDays, "day", "days")))
	}
	if res.C.Stale {
		parts = append(parts, fmt.Sprintf("%s open for more than %d days without a verified fix (oldest %d days)",
			plural(len(res.C.StaleIDs), "complaint", "complaints"), r.ComplaintCluster.StaleOpenDays, res.C.OldestStaleDays))
	}
	if res.T.Fired {
		parts = append(parts, testPhraseEN(res.T.Failing))
	}
	if res.I.CaseFired {
		parts = append(parts, fmt.Sprintf("%d diarrhoea/vomiting cases reported in the last %d h", res.I.Cases, r.IllnessCluster.WindowHours))
	}
	for _, s := range res.I.ORSSpikes {
		parts = append(parts, fmt.Sprintf("ORS sales at a reporting chemist at %.1fx its 4-week average (%d vs %.1f units/day)", s.Ratio, s.Units, s.Baseline))
	}
	return fmt.Sprintf("%s — %s: %s. %s", res.Level.Title(), ward, strings.Join(parts, "; "), countPhraseEN(res))
}

func testPhraseEN(fs []TestSignal) string {
	if len(fs) == 1 {
		t := fs[0]
		detail := ""
		if t.Value != nil && t.Limit != nil {
			detail = fmt.Sprintf(" (%g %s vs limit %g)", *t.Value, t.Unit, *t.Limit)
		}
		return fmt.Sprintf("a %s-positive sample on %s%s has no passing retest", t.Parameter, day(t.SampledAt), detail)
	}
	params := map[string]bool{}
	var names []string
	for _, t := range fs {
		if !params[normParam(t.Parameter)] {
			params[normParam(t.Parameter)] = true
			names = append(names, t.Parameter)
		}
	}
	return fmt.Sprintf("%d failing samples (%s), latest on %s, have no passing retest", len(fs), strings.Join(names, ", "), day(fs[len(fs)-1].SampledAt))
}

func countPhraseEN(res Result) string {
	switch res.SignalCount {
	case 3:
		return "All three signals."
	case 2:
		if res.Level == domain.LevelCritical {
			return fmt.Sprintf("Two signals of three, with %d illness cases.", res.I.Cases)
		}
		return "Two signals of three."
	}
	return "One signal of three."
}

// roundUp10 rounds a distance up to the next 10 m (ignoring sub-metre noise)
// so reason text never understates how spread out a cluster is.
func roundUp10(m float64) int { return int(math.Ceil((m-0.5)/10)) * 10 }

func reasonHI(r *Rule, ward string, res Result) string {
	lvl := map[domain.Level]string{domain.LevelWatch: "निगरानी", domain.LevelWarning: "चेतावनी", domain.LevelCritical: "गंभीर"}[res.Level]
	var parts []string
	if res.C.Cluster {
		parts = append(parts, fmt.Sprintf("%d मीटर के भीतर %d घरों ने %d दिनों में %s की शिकायत की",
			roundUp10(res.C.ClusterSpanM), res.C.ClusterHouseholds, res.C.ClusterDays, issuePhraseHI(res.C.ClusterIssue)))
	}
	if res.C.Stale {
		parts = append(parts, fmt.Sprintf("%d शिकायतें %d दिन से अधिक समय से बिना सत्यापित सुधार के खुली हैं (सबसे पुरानी %d दिन)",
			len(res.C.StaleIDs), r.ComplaintCluster.StaleOpenDays, res.C.OldestStaleDays))
	}
	if res.T.Fired {
		t := res.T.Failing[len(res.T.Failing)-1]
		parts = append(parts, fmt.Sprintf("%d पानी के नमूने दूषित पाए गए (%s, %s), दोबारा जाँच में साफ़ नहीं निकले", len(res.T.Failing), t.Parameter, day(t.SampledAt)))
	}
	if res.I.CaseFired {
		parts = append(parts, fmt.Sprintf("पिछले %d घंटों में दस्त/उल्टी के %d मामले", r.IllnessCluster.WindowHours, res.I.Cases))
	}
	if len(res.I.ORSSpikes) > 0 {
		parts = append(parts, fmt.Sprintf("एक दवा दुकान पर ORS की बिक्री सामान्य से %.1f गुना", res.I.ORSSpikes[0].Ratio))
	}
	count := map[int]string{1: "तीन में से एक संकेत।", 2: "तीन में से दो संकेत।", 3: "तीनों संकेत।"}[res.SignalCount]
	return fmt.Sprintf("%s — %s: %s। %s", lvl, ward, strings.Join(parts, "; "), count)
}

func issuePhraseHI(it domain.IssueType) string {
	switch it {
	case domain.IssueDirtyWater:
		return "गंदे पानी"
	case domain.IssueFoulSmell:
		return "बदबूदार पानी"
	case domain.IssueSewageMixing:
		return "पीने के पानी में सीवेज मिलने"
	}
	return "गंदे/बदबूदार/सीवेज मिले पानी"
}

func distance(a, b domain.Point) float64 { return geo.DistanceM(a, b) }
