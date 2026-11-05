package rule

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

func randomSnapshot(rng *rand.Rand) Snapshot {
	var s Snapshot
	for i := 0; i < rng.IntN(12); i++ {
		c := cmp(fmt.Sprint("c", i), fmt.Sprint("r", rng.IntN(6)), []domain.IssueType{domain.IssueDirtyWater, domain.IssueFoulSmell, domain.IssueSewageMixing, domain.IssueLeak}[rng.IntN(4)],
			float64(rng.IntN(24*20)), nil)
		if rng.IntN(3) > 0 {
			c.Point = pt(float64(rng.IntN(600)), float64(rng.IntN(600)))
		}
		if rng.IntN(5) == 0 {
			c.Reporter = ""
		}
		if rng.IntN(6) == 0 {
			f := c.At.Add(time.Duration(rng.IntN(200)) * time.Hour)
			c.VerifiedFixedAt = &f
		}
		s.Complaints = append(s.Complaints, c)
	}
	for i := 0; i < rng.IntN(4); i++ {
		s.Tests = append(s.Tests, TestSignal{ID: fmt.Sprint("t", i), SampledAt: ago(float64(rng.IntN(24 * 40))), Parameter: []string{"E. coli", "pH"}[rng.IntN(2)], Contaminated: rng.IntN(2) == 0})
	}
	for i := 0; i < rng.IntN(6); i++ {
		s.Illness = append(s.Illness, IllnessSignal{ID: fmt.Sprint("i", i), Reporter: "clinic", At: ago(float64(rng.IntN(24 * 6))), Diarrhoea: rng.IntN(3), Vomiting: rng.IntN(3)})
	}
	return s
}

// TestRuleMonotonicity: adding risk evidence can never lower the level and
// confirming a fix can never raise it.
func TestRuleMonotonicity(t *testing.T) {
	r := load(t)
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 4000; trial++ {
		s := randomSnapshot(rng)
		base := r.Evaluate(s, now).Level

		more := s
		more.Tests = append(append([]TestSignal{}, s.Tests...), TestSignal{ID: "x", SampledAt: ago(float64(rng.IntN(24 * 29))), Parameter: "Nitrate", Contaminated: true})
		if l := r.Evaluate(more, now).Level; l < base {
			t.Fatalf("trial %d: adding a failed test lowered %v → %v", trial, base, l)
		}
		ill := s
		ill.Illness = append(append([]IllnessSignal{}, s.Illness...), IllnessSignal{ID: "y", Reporter: "c2", At: ago(5), Diarrhoea: 1 + rng.IntN(4)})
		if l := r.Evaluate(ill, now).Level; l < base {
			t.Fatalf("trial %d: adding illness lowered %v → %v", trial, base, l)
		}
		cmpl := s
		cmpl.Complaints = append(append([]ComplaintSignal{}, s.Complaints...), cmp("z", "rz", domain.IssueDirtyWater, float64(rng.IntN(24*20)), pt(float64(rng.IntN(600)), float64(rng.IntN(600)))))
		if l := r.Evaluate(cmpl, now).Level; l < base {
			t.Fatalf("trial %d: adding a complaint lowered %v → %v", trial, base, l)
		}
		if len(s.Complaints) > 0 {
			fixed := s
			fixed.Complaints = append([]ComplaintSignal{}, s.Complaints...)
			k := rng.IntN(len(fixed.Complaints))
			f := now.Add(-time.Minute)
			fixed.Complaints[k].VerifiedFixedAt = &f
			if l := r.Evaluate(fixed, now).Level; l > base {
				t.Fatalf("trial %d: verifying a fix raised %v → %v", trial, base, l)
			}
		}
	}
}

// TestRuleTimeTranslationInvariance: shifting every event and the evaluation
// time by the same amount gives the same verdict (no hidden wall-clock use).
func TestRuleTimeTranslationInvariance(t *testing.T) {
	r := load(t)
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := 0; trial < 2000; trial++ {
		s := randomSnapshot(rng)
		shift := time.Duration(rng.IntN(24*365*3)) * time.Hour
		moved := Snapshot{}
		for _, c := range s.Complaints {
			c.At = c.At.Add(shift)
			if c.VerifiedFixedAt != nil {
				f := c.VerifiedFixedAt.Add(shift)
				c.VerifiedFixedAt = &f
			}
			moved.Complaints = append(moved.Complaints, c)
		}
		for _, x := range s.Tests {
			x.SampledAt = x.SampledAt.Add(shift)
			moved.Tests = append(moved.Tests, x)
		}
		for _, x := range s.Illness {
			x.At = x.At.Add(shift)
			moved.Illness = append(moved.Illness, x)
		}
		a, b := r.Evaluate(s, now), r.Evaluate(moved, now.Add(shift))
		if a.Level != b.Level || a.SignalCount != b.SignalCount || a.C.Fired != b.C.Fired || a.T.Fired != b.T.Fired || a.I.Fired != b.I.Fired {
			t.Fatalf("trial %d: verdict changed under time shift %v: %v vs %v", trial, shift, a.Level, b.Level)
		}
	}
}

// TestRuleInputOrderIrrelevant: shuffling input slices never changes the level.
func TestRuleInputOrderIrrelevant(t *testing.T) {
	r := load(t)
	rng := rand.New(rand.NewPCG(5, 6))
	for trial := 0; trial < 2000; trial++ {
		s := randomSnapshot(rng)
		sh := Snapshot{Complaints: append([]ComplaintSignal{}, s.Complaints...), Tests: append([]TestSignal{}, s.Tests...), Illness: append([]IllnessSignal{}, s.Illness...)}
		rng.Shuffle(len(sh.Complaints), func(i, j int) { sh.Complaints[i], sh.Complaints[j] = sh.Complaints[j], sh.Complaints[i] })
		rng.Shuffle(len(sh.Tests), func(i, j int) { sh.Tests[i], sh.Tests[j] = sh.Tests[j], sh.Tests[i] })
		rng.Shuffle(len(sh.Illness), func(i, j int) { sh.Illness[i], sh.Illness[j] = sh.Illness[j], sh.Illness[i] })
		if a, b := r.Evaluate(s, now), r.Evaluate(sh, now); a.Level != b.Level || a.C.ClusterHouseholds != b.C.ClusterHouseholds {
			t.Fatalf("trial %d: input order changed the verdict", trial)
		}
	}
}

func FuzzParseRule(f *testing.F) {
	f.Add([]byte("version: v1\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := Parse(b)
		if err == nil && (r.Version == "" || len(r.SHA256) != 64) {
			t.Fatal("accepted an invalid rule")
		}
	})
}
