package backtest

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/rule"
)

const sample = `event_id,kind,in_ward,date_earliest,date_latest,count,issue_type,parameter,contaminated,value,limit,unit,diarrhoea,vomiting,source_url
C1,complaint,yes,2025-10-01,2025-10-10,3,dirty_water,,,,,,,,u
C2,complaint,no,2025-09-01,2025-09-01,50,dirty_water,,,,,,,,u
T1,test,yes,,2025-12-01,,,E. coli,true,,,,,,u
T2,test,yes,2025-12-05,2025-12-05,,,e. coli,false,,,,,,u
I1,illness,yes,2025-12-20,2025-12-23,,,,,,,,8,2,u
I2,illness,yes,2025-12-20,2025-12-20,40,,,,,,,,,u
`

func rows(t *testing.T) []Row {
	rs, err := ReadCSV(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestReadCSVValidates(t *testing.T) {
	rs := rows(t)
	if len(rs) != 6 || rs[2].Earliest != rs[2].Latest || rs[1].InWard {
		t.Fatalf("parsed wrong: %+v", rs)
	}
	bad := strings.Replace(sample, "2025-10-01,2025-10-10", "2025-10-11,2025-10-10", 1)
	if _, err := ReadCSV(strings.NewReader(bad)); err == nil {
		t.Fatal("earliest after latest accepted")
	}
	if _, err := ReadCSV(strings.NewReader("event_id,kind\nX,complaint\n")); err == nil {
		t.Fatal("missing columns accepted")
	}
}

func TestScenarioPlacement(t *testing.T) {
	o := Outbreak{Slug: "x", Name: "X"}
	best := Snapshot(o, rows(t), Best)
	cons := Snapshot(o, rows(t), Conservative)
	if len(best.Complaints) != 3 || len(cons.Complaints) != 3 {
		t.Fatalf("complaints: %d %d (out-of-ward rows must be dropped)", len(best.Complaints), len(cons.Complaints))
	}
	if !best.Complaints[0].At.Before(best.Complaints[2].At) || !cons.Complaints[0].At.Equal(cons.Complaints[2].At) {
		t.Fatal("best must spread, conservative must stack at the latest date")
	}
	total := func(s rule.Snapshot) int {
		n := 0
		for _, i := range s.Illness {
			n += i.Diarrhoea
		}
		return n
	}
	if total(best) != 8 || total(cons) != 8 {
		t.Fatalf("illness cases: best %d cons %d; want 8 (max(d,v); row without counts ignored)", total(best), total(cons))
	}
	for _, c := range append(best.Complaints, cons.Complaints...) {
		if c.Point != nil {
			t.Fatal("backtest complaints must not be geolocated")
		}
	}
}

func TestReplayIsDeterministicAndOrdered(t *testing.T) {
	r, err := rule.Load("../../config/alert_rule_v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	h := map[domain.Level]int{domain.LevelWatch: 72, domain.LevelWarning: 48, domain.LevelCritical: 24}
	o := Outbreak{Slug: "x", Name: "X"}
	for _, s := range []Scenario{Best, Conservative} {
		a := Replay(r, o, rows(t), s, h)
		b := Replay(r, o, rows(t), s, h)
		if a.FirstAt[domain.LevelWatch] != b.FirstAt[domain.LevelWatch] {
			t.Fatal("non-deterministic")
		}
		w := a.FirstAt[domain.LevelWatch]
		if w.IsZero() {
			t.Fatal("stale complaints must raise Watch")
		}
		// Watch comes from complaints open > 7 days: never before 7 days after the first one.
		first := a.FirstSignal["C"]
		if s == Conservative && first.Sub(time.Date(2025, 10, 17, 6, 30, 0, 0, time.UTC)) < 0 {
			t.Fatalf("conservative C fired too early: %v", first)
		}
		if a.CommissionerBy.Before(w) {
			t.Fatal("commissioner reached before the alert")
		}
	}
	cons := Replay(r, o, rows(t), Conservative, h)
	best := Replay(r, o, rows(t), Best, h)
	if cons.FirstAt[domain.LevelWatch].Before(best.FirstAt[domain.LevelWatch]) {
		t.Fatal("conservative fired earlier than best")
	}
}

func TestVariantsOnlyRemoveEvidence(t *testing.T) {
	rs := rows(t)
	if n := len(Filter(rs, Variant{Exclude: []string{"C1"}})); n != 5 {
		t.Fatalf("exclude: %d", n)
	}
	f := Filter(rs, Variant{OnlyComplaints: []string{"C2"}})
	for _, r := range f {
		if r.ID == "C1" {
			t.Fatal("only_complaints kept another complaint")
		}
	}
	if len(f) != 5 {
		t.Fatalf("only_complaints must keep non-complaint rows: %d", len(f))
	}
}

// TestCommittedResultsAreReproducible regenerates RESULTS.md from the
// committed data and the frozen rule and compares it byte for byte.
func TestCommittedResultsAreReproducible(t *testing.T) {
	r, err := rule.Load("../../config/alert_rule_v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckFrozen(); err != nil {
		t.Fatal(err)
	}
	obs, vs, err := LoadDir("../../backtest")
	if err != nil {
		t.Fatal(err)
	}
	h := map[domain.Level]int{domain.LevelWatch: 72, domain.LevelWarning: 48, domain.LevelCritical: 24}
	got := Markdown(r, obs, vs, Run(r, obs, vs, h))
	want, err := os.ReadFile("../../backtest/RESULTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("backtest/RESULTS.md is stale or hand-edited; run `go run ./cmd/bhagirath backtest` and commit the result")
	}
}
