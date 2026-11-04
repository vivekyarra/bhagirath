package rule

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func load(t *testing.T) *Rule {
	t.Helper()
	r, err := Load("../../config/alert_rule_v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ago(h float64) time.Time { return now.Add(-time.Duration(h * float64(time.Hour))) }

// pt returns a point dy metres north and dx metres east of a base point.
func pt(dyM, dxM float64) *domain.Point {
	return &domain.Point{Lat: 22.72 + dyM/111_195, Lon: 75.86 + dxM/(111_195*0.9223)}
}

func cmp(id, reporter string, it domain.IssueType, hoursAgo float64, p *domain.Point) ComplaintSignal {
	return ComplaintSignal{ID: id, Reporter: reporter, IssueType: it, At: ago(hoursAgo), Point: p}
}

func cluster3() []ComplaintSignal {
	return []ComplaintSignal{
		cmp("c1", "r1", domain.IssueFoulSmell, 24*6, pt(0, 0)),
		cmp("c2", "r2", domain.IssueFoulSmell, 24*4, pt(100, 50)),
		cmp("c3", "r3", domain.IssueDirtyWater, 24*1, pt(200, 0)),
	}
}

func failingTest(hoursAgo float64) TestSignal {
	v, l := 12.0, 0.0
	return TestSignal{ID: "t1", SampledAt: ago(hoursAgo), Parameter: "E. coli", Contaminated: true, Value: &v, Limit: &l, Unit: "MPN/100ml"}
}

func illness(id string, hoursAgo float64, d, v int) IllnessSignal {
	return IllnessSignal{ID: id, Reporter: "clinic1", At: ago(hoursAgo), Diarrhoea: d, Vomiting: v}
}

func TestNoSignalsNoAlert(t *testing.T) {
	res := load(t).Evaluate(Snapshot{WardID: "w", WardName: "Ward 12"}, now)
	if res.Level != domain.LevelNone || res.Reason != "" {
		t.Fatalf("got %v %q", res.Level, res.Reason)
	}
}

func TestWatchFromComplaintCluster(t *testing.T) {
	res := load(t).Evaluate(Snapshot{WardName: "Ward 12", Complaints: cluster3()}, now)
	if res.Level != domain.LevelWatch || !res.C.Cluster || res.C.Stale {
		t.Fatalf("level=%v C=%+v", res.Level, res.C)
	}
	if res.C.ClusterHouseholds != 3 || res.C.ClusterSpanM > 201 || res.C.ClusterSpanM < 190 {
		t.Fatalf("cluster detail wrong: %+v", res.C)
	}
	want := "Watch — Ward 12: 3 households within 200 m reported dirty, foul-smelling or sewage-mixed water in 5 days"
	if !strings.HasPrefix(res.Reason, want) || !strings.HasSuffix(res.Reason, "One signal of three.") {
		t.Fatalf("reason = %q", res.Reason)
	}
	if !strings.Contains(res.ReasonHI, "निगरानी") {
		t.Fatalf("hindi reason = %q", res.ReasonHI)
	}
}

func TestClusterRequiresPairwiseRadius(t *testing.T) {
	cs := []ComplaintSignal{
		cmp("c1", "r1", domain.IssueFoulSmell, 24, pt(0, 0)),
		cmp("c2", "r2", domain.IssueFoulSmell, 24, pt(250, 0)),
		cmp("c3", "r3", domain.IssueFoulSmell, 24, pt(-250, 0)), // 500 m from c2
	}
	res := load(t).Evaluate(Snapshot{Complaints: cs}, now)
	if res.C.Cluster {
		t.Fatalf("chain of 250 m hops must not form a 300 m pairwise cluster: %+v", res.C)
	}
}

func TestClusterNeedsDistinctReporters(t *testing.T) {
	cs := cluster3()
	cs[2].Reporter = "r1"
	if res := load(t).Evaluate(Snapshot{Complaints: cs}, now); res.C.Cluster {
		t.Fatal("two reporters formed a cluster")
	}
	// Anonymous complaints collapse to one reporter.
	for i := range cs {
		cs[i].Reporter = ""
	}
	if res := load(t).Evaluate(Snapshot{Complaints: cs}, now); res.C.Cluster {
		t.Fatal("three anonymous complaints formed a cluster")
	}
}

func TestClusterFindsValidSubsetAmongNoise(t *testing.T) {
	// Same-reporter repeats and far-away complaints must not hide a real cluster.
	cs := []ComplaintSignal{
		cmp("a1", "r1", domain.IssueSewageMixing, 30, pt(0, 0)),
		cmp("a2", "r1", domain.IssueSewageMixing, 29, pt(10, 10)),
		cmp("a3", "r1", domain.IssueSewageMixing, 28, pt(20, 0)),
		cmp("far", "r9", domain.IssueSewageMixing, 27, pt(5000, 0)),
		cmp("b1", "r2", domain.IssueSewageMixing, 26, pt(50, 50)),
		cmp("b2", "r3", domain.IssueSewageMixing, 25, pt(80, -40)),
		cmp("nowater", "r4", domain.IssueNoWater, 25, pt(1, 1)),
	}
	res := load(t).Evaluate(Snapshot{Complaints: cs}, now)
	if !res.C.Cluster || res.C.ClusterHouseholds != 3 || res.C.ClusterIssue != domain.IssueSewageMixing {
		t.Fatalf("C = %+v", res.C)
	}
	for _, id := range res.C.ClusterIDs {
		if id == "far" || id == "nowater" {
			t.Fatalf("cluster contains %s", id)
		}
	}
}

func TestClusterWindowAndExclusions(t *testing.T) {
	r := load(t)
	cs := cluster3()
	cs[0].At = ago(24*14 + 1) // just outside 14 days — but now stale (open > 7 days)
	res := r.Evaluate(Snapshot{Complaints: cs}, now)
	if res.C.Cluster || !res.C.Stale {
		t.Fatalf("window edge: %+v", res.C)
	}
	cs = cluster3()
	cs[1].Point = nil // unlocated never counts toward a cluster
	if res := r.Evaluate(Snapshot{Complaints: cs}, now); res.C.Cluster {
		t.Fatal("unlocated complaint counted toward cluster")
	}
	cs = cluster3()
	cs[1].Invalid = true
	if res := r.Evaluate(Snapshot{Complaints: cs}, now); res.C.Fired {
		t.Fatal("invalid complaint counted")
	}
	cs = cluster3()
	fixed := ago(1)
	cs[2].VerifiedFixedAt = &fixed
	cs[0].VerifiedFixedAt = &fixed
	if res := r.Evaluate(Snapshot{Complaints: cs}, now); res.C.Cluster {
		t.Fatal("verified-fixed complaints counted toward cluster")
	}
	// No look-ahead: a complaint after `now` is ignored.
	cs = cluster3()
	cs[2].At = now.Add(time.Hour)
	if res := r.Evaluate(Snapshot{Complaints: cs}, now); res.C.Cluster {
		t.Fatal("future complaint used")
	}
}

func TestStaleComplaintAloneIsWatch(t *testing.T) {
	r := load(t)
	c := cmp("old", "r1", domain.IssueDirtyWater, 24*7+1, nil)
	res := r.Evaluate(Snapshot{WardName: "W", Complaints: []ComplaintSignal{c}}, now)
	if res.Level != domain.LevelWatch || !res.C.Stale || res.C.OldestStaleDays != 7 {
		t.Fatalf("got %v %+v", res.Level, res.C)
	}
	if !strings.Contains(res.Reason, "1 complaint open for more than 7 days without a verified fix") {
		t.Fatalf("reason = %q", res.Reason)
	}
	c.At = ago(24*7 - 1)
	if res := r.Evaluate(Snapshot{Complaints: []ComplaintSignal{c}}, now); res.C.Stale {
		t.Fatal("complaint under 7 days counted stale")
	}
	// Verified fix before now clears it; a fix verified AFTER now does not (backtest look-ahead guard).
	c.At = ago(24 * 30)
	later := now.Add(time.Hour)
	c.VerifiedFixedAt = &later
	if res := r.Evaluate(Snapshot{Complaints: []ComplaintSignal{c}}, now); !res.C.Stale {
		t.Fatal("future verification used")
	}
	earlier := ago(1)
	c.VerifiedFixedAt = &earlier
	if res := r.Evaluate(Snapshot{Complaints: []ComplaintSignal{c}}, now); res.C.Stale {
		t.Fatal("verified complaint still stale")
	}
	// Non-water issue types never count.
	c = cmp("leak", "r1", domain.IssueLeak, 24*30, nil)
	if res := r.Evaluate(Snapshot{Complaints: []ComplaintSignal{c}}, now); res.C.Fired {
		t.Fatal("leak complaint counted toward C")
	}
}

func TestWatchFromTestFailure(t *testing.T) {
	r := load(t)
	res := r.Evaluate(Snapshot{WardName: "W", Tests: []TestSignal{failingTest(48)}}, now)
	if res.Level != domain.LevelWatch || !res.T.Fired {
		t.Fatalf("got %v", res.Level)
	}
	if !strings.Contains(res.Reason, "E. coli-positive sample on 18 Nov 2026 (12 MPN/100ml vs limit 0) has no passing retest") {
		t.Fatalf("reason = %q", res.Reason)
	}
	// A later passing retest of the same parameter clears it.
	pass := TestSignal{ID: "t2", SampledAt: ago(10), Parameter: "e. coli ", Contaminated: false}
	if res := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(48), pass}}, now); res.T.Fired {
		t.Fatal("retest did not clear")
	}
	// A passing test of a different parameter does not.
	other := TestSignal{ID: "t3", SampledAt: ago(10), Parameter: "pH", Contaminated: false}
	if res := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(48), other}}, now); !res.T.Fired {
		t.Fatal("different parameter cleared failure")
	}
	// An earlier passing test does not clear a later failure.
	early := TestSignal{ID: "t4", SampledAt: ago(100), Parameter: "E. coli", Contaminated: false}
	if res := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(48), early}}, now); !res.T.Fired {
		t.Fatal("earlier pass cleared later failure")
	}
	// Older than 30 days is out of window.
	if res := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(24*30 + 1)}}, now); res.T.Fired {
		t.Fatal("31-day-old failure counted")
	}
	// Retest in the future is not visible yet.
	future := TestSignal{ID: "t5", SampledAt: now.Add(time.Hour), Parameter: "E. coli"}
	if res := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(48), future}}, now); !res.T.Fired {
		t.Fatal("future retest used")
	}
}

func TestWatchFromIllness(t *testing.T) {
	r := load(t)
	res := r.Evaluate(Snapshot{Illness: []IllnessSignal{illness("i1", 10, 2, 1), illness("i2", 30, 1, 0)}}, now)
	if res.Level != domain.LevelWatch || res.I.Cases != 3 {
		t.Fatalf("got %v cases=%d", res.Level, res.I.Cases)
	}
	// max(d, v) per report, not the sum: 2 diarrhoea + 2 vomiting = 2 cases.
	res = r.Evaluate(Snapshot{Illness: []IllnessSignal{illness("i1", 10, 2, 2)}}, now)
	if res.I.Fired || res.I.Cases != 2 {
		t.Fatalf("sum used instead of max: %+v", res.I)
	}
	// Outside 72 h.
	res = r.Evaluate(Snapshot{Illness: []IllnessSignal{illness("i1", 73, 5, 5)}}, now)
	if res.I.Fired {
		t.Fatal("old illness counted")
	}
}

func TestORSSpike(t *testing.T) {
	r := load(t)
	ors := func(id string, hoursAgo float64, n int) IllnessSignal {
		v := n
		return IllnessSignal{ID: id, Reporter: "chem1", At: ago(hoursAgo), ORS: &v}
	}
	var reps []IllnessSignal
	for d := 4; d <= 24; d += 2 { // 11 baseline days at 10 units
		reps = append(reps, ors(fmt.Sprintf("b%d", d), float64(24*d), 10))
	}
	spike := append(append([]IllnessSignal{}, reps...), ors("now", 5, 20))
	res := r.Evaluate(Snapshot{WardName: "W", Illness: spike}, now)
	if !res.I.Fired || len(res.I.ORSSpikes) != 1 || res.I.ORSSpikes[0].Ratio != 2 {
		t.Fatalf("I = %+v", res.I)
	}
	if !strings.Contains(res.Reason, "ORS sales at a reporting chemist at 2.0x") {
		t.Fatalf("reason = %q", res.Reason)
	}
	noSpike := append(append([]IllnessSignal{}, reps...), ors("now", 5, 19))
	if res := r.Evaluate(Snapshot{Illness: noSpike}, now); res.I.Fired {
		t.Fatal("1.9x fired")
	}
	// Too little history: no baseline, no spike.
	thin := []IllnessSignal{ors("a", 48, 1), ors("b", 72, 1), ors("now", 5, 50)}
	if res := r.Evaluate(Snapshot{Illness: thin}, now); res.I.Fired {
		t.Fatal("spike fired without 7 baseline reports")
	}
	// Zero baseline never fires.
	var zeros []IllnessSignal
	for d := 4; d <= 24; d += 2 {
		zeros = append(zeros, ors(fmt.Sprintf("z%d", d), float64(24*d), 0))
	}
	zeros = append(zeros, ors("now", 5, 3))
	if res := r.Evaluate(Snapshot{Illness: zeros}, now); res.I.Fired {
		t.Fatal("zero baseline fired")
	}
}

func TestWarningFromTwoSignals(t *testing.T) {
	r := load(t)
	res := r.Evaluate(Snapshot{WardName: "Ward 12", Complaints: cluster3(), Tests: []TestSignal{failingTest(24 * 3)}}, now)
	if res.Level != domain.LevelWarning || res.SignalCount != 2 {
		t.Fatalf("got %v", res.Level)
	}
	if !strings.HasPrefix(res.Reason, "Warning — Ward 12: 3 households within") || !strings.HasSuffix(res.Reason, "Two signals of three.") {
		t.Fatalf("reason = %q", res.Reason)
	}
	// C + I with only 3 cases is Warning, not Critical.
	res = r.Evaluate(Snapshot{Complaints: cluster3(), Illness: []IllnessSignal{illness("i", 5, 3, 0)}}, now)
	if res.Level != domain.LevelWarning {
		t.Fatalf("C+I(3) = %v", res.Level)
	}
	// T + I with 4 cases is Warning.
	res = r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(5)}, Illness: []IllnessSignal{illness("i", 5, 4, 0)}}, now)
	if res.Level != domain.LevelWarning {
		t.Fatalf("T+I(4) = %v", res.Level)
	}
	// C + T (no illness) stays Warning regardless.
	res = r.Evaluate(Snapshot{Complaints: cluster3(), Tests: []TestSignal{failingTest(5)}}, now)
	if res.Level != domain.LevelWarning {
		t.Fatalf("C+T = %v", res.Level)
	}
}

func TestCritical(t *testing.T) {
	r := load(t)
	all3 := r.Evaluate(Snapshot{WardName: "W", Complaints: cluster3(), Tests: []TestSignal{failingTest(5)}, Illness: []IllnessSignal{illness("i", 5, 3, 0)}}, now)
	if all3.Level != domain.LevelCritical || !strings.HasSuffix(all3.Reason, "All three signals.") {
		t.Fatalf("all three = %v %q", all3.Level, all3.Reason)
	}
	ci5 := r.Evaluate(Snapshot{WardName: "W", Complaints: cluster3(), Illness: []IllnessSignal{illness("i", 5, 5, 1)}}, now)
	if ci5.Level != domain.LevelCritical || !strings.HasSuffix(ci5.Reason, "Two signals of three, with 5 illness cases.") {
		t.Fatalf("C+I(5) = %v %q", ci5.Level, ci5.Reason)
	}
	ti5 := r.Evaluate(Snapshot{Tests: []TestSignal{failingTest(5)}, Illness: []IllnessSignal{illness("i", 5, 0, 5)}}, now)
	if ti5.Level != domain.LevelCritical {
		t.Fatalf("T+I(5) = %v", ti5.Level)
	}
	// Illness alone with many cases is only Watch.
	i9 := r.Evaluate(Snapshot{Illness: []IllnessSignal{illness("i", 5, 9, 9)}}, now)
	if i9.Level != domain.LevelWatch {
		t.Fatalf("I(9) alone = %v", i9.Level)
	}
}

func TestDeterministic(t *testing.T) {
	r := load(t)
	s := Snapshot{WardName: "W", Complaints: cluster3(), Tests: []TestSignal{failingTest(5)}}
	a, b := r.Evaluate(s, now), r.Evaluate(s, now)
	if a.Reason != b.Reason || a.Level != b.Level || strings.Join(a.C.ClusterIDs, ",") != strings.Join(b.C.ClusterIDs, ",") {
		t.Fatal("non-deterministic result")
	}
}

func TestParseRejectsUnknownKeysAndBadValues(t *testing.T) {
	if _, err := Parse([]byte("version: v1\nsurprise: 1\n")); err == nil {
		t.Fatal("unknown key accepted")
	}
	r := load(t)
	if r.Version != "v1" || len(r.SHA256) != 64 || r.LookbackDays() < 30 {
		t.Fatalf("rule = %+v", r)
	}
}

func BenchmarkClusterSearch(b *testing.B) {
	r, _ := Load("../../config/alert_rule_v1.yaml")
	var cs []ComplaintSignal
	for i := 0; i < 300; i++ {
		cs = append(cs, cmp(fmt.Sprint(i), fmt.Sprint("r", i%120), domain.IssueDirtyWater, float64(i%300), pt(float64(i%17)*90, float64(i%13)*90)))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Evaluate(Snapshot{Complaints: cs}, now)
	}
}

func TestClusterSearchScalesUnderStress(t *testing.T) {
	r := load(t)
	var cs []ComplaintSignal
	seed := uint32(7)
	rnd := func() float64 { seed = seed*1664525 + 1013904223; return float64(seed%10000) / 10000 }
	for i := 0; i < 1500; i++ {
		cs = append(cs, cmp(fmt.Sprint(i), fmt.Sprint("r", i%400), domain.IssueDirtyWater, rnd()*300, pt(rnd()*1500, rnd()*1500)))
	}
	start := time.Now()
	res := r.Evaluate(Snapshot{Complaints: cs}, now)
	if el := time.Since(start); el > 30*time.Second { // generous: CI runs with -race
		t.Fatalf("1500-complaint ward took %v", el)
	}
	if !res.C.Cluster {
		t.Fatal("dense ward produced no cluster")
	}
	for i, a := range res.C.ClusterIDs {
		for _, b := range res.C.ClusterIDs[i+1:] {
			var pa, pb *domain.Point
			for _, c := range cs {
				if c.ID == a {
					pa = c.Point
				}
				if c.ID == b {
					pb = c.Point
				}
			}
			if d := distance(*pa, *pb); d > 300 {
				t.Fatalf("cluster members %s,%s are %.0f m apart", a, b, d)
			}
		}
	}
}

func mustRead(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../config/alert_rule_v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestClusterExistenceMatchesBruteForce checks the fast cluster search against
// an exhaustive O(n³) search on thousands of random small wards.
func TestClusterExistenceMatchesBruteForce(t *testing.T) {
	r := load(t)
	seed := uint64(42)
	rnd := func(n int) int { seed = seed*6364136223846793005 + 1442695040888963407; return int((seed >> 33) % uint64(n)) }
	for trial := 0; trial < 3000; trial++ {
		n := 3 + rnd(9)
		var cs []ComplaintSignal
		for i := 0; i < n; i++ {
			rep := fmt.Sprint("r", rnd(4))
			if rnd(6) == 0 {
				rep = ""
			}
			cs = append(cs, cmp(fmt.Sprint(i), rep, domain.IssueDirtyWater, float64(rnd(24*6)), pt(float64(rnd(700)), float64(rnd(700)))))
		}
		got := r.Evaluate(Snapshot{Complaints: cs}, now).C.Cluster
		want := false
		key := func(c ComplaintSignal) string {
			if c.Reporter == "" {
				return "anon"
			}
			return c.Reporter
		}
		for i := 0; i < n && !want; i++ {
			for j := i + 1; j < n && !want; j++ {
				for k := j + 1; k < n && !want; k++ {
					a, b, c := cs[i], cs[j], cs[k]
					if distance(*a.Point, *b.Point) <= 300 && distance(*a.Point, *c.Point) <= 300 && distance(*b.Point, *c.Point) <= 300 &&
						key(a) != key(b) && key(a) != key(c) && key(b) != key(c) {
						want = true
					}
				}
			}
		}
		if got != want {
			t.Fatalf("trial %d: fast=%v brute=%v", trial, got, want)
		}
	}
}
