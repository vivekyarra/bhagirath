// Package rule is the deterministic alert engine (spec Section 8). The same
// Evaluate function runs every 15 minutes in production and replays history
// in the backtest, so the published backtest is exactly what would have run.
package rule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

// Rule is a parsed, validated alert rule.
type Rule struct {
	Version          string           `yaml:"version"`
	ComplaintCluster ComplaintCluster `yaml:"complaint_cluster"`
	TestFailure      TestFailure      `yaml:"test_failure"`
	IllnessCluster   IllnessCluster   `yaml:"illness_cluster"`
	Levels           Levels           `yaml:"levels"`

	// SHA256 is the hex digest of the exact file bytes the rule came from.
	SHA256 string `yaml:"-"`
}

// ComplaintCluster configures signal C.
type ComplaintCluster struct {
	IssueTypes                  []domain.IssueType `yaml:"issue_types"`
	WindowDays                  int                `yaml:"window_days"`
	MinComplaints               int                `yaml:"min_complaints"`
	MinDistinctReporters        int                `yaml:"min_distinct_reporters"`
	PairwiseRadiusM             float64            `yaml:"pairwise_radius_m"`
	UnlocatedCountTowardCluster bool               `yaml:"unlocated_count_toward_cluster"`
	AnonymousReportersAreOne    bool               `yaml:"anonymous_reporters_are_one"`
	StaleOpenDays               int                `yaml:"stale_open_days"`
	ExcludeVerifiedFixed        bool               `yaml:"exclude_verified_fixed"`
}

// TestFailure configures signal T.
type TestFailure struct {
	WindowDays          int  `yaml:"window_days"`
	RetestSameParameter bool `yaml:"retest_same_parameter"`
}

// IllnessCluster configures signal I.
type IllnessCluster struct {
	WindowHours    int    `yaml:"window_hours"`
	MinCases       int    `yaml:"min_cases"`
	CasesPerReport string `yaml:"cases_per_report"` // max | sum
	ORS            ORS    `yaml:"ors"`
}

// ORS configures the chemist ORS-sales spike sub-signal.
type ORS struct {
	Multiplier         float64 `yaml:"multiplier"`
	BaselineDays       int     `yaml:"baseline_days"`
	MinBaselineReports int     `yaml:"min_baseline_reports"`
	WindowHours        int     `yaml:"window_hours"`
}

// Levels configures level assignment.
type Levels struct {
	CriticalMinIllnessCases int `yaml:"critical_min_illness_cases"`
}

// Load reads and validates a rule file.
func Load(path string) (*Rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse validates rule bytes (unknown keys are rejected).
func Parse(b []byte) (*Rule, error) {
	var r Rule
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("parse rule: %w", err)
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	r.SHA256 = hex.EncodeToString(sum[:])
	return &r, nil
}

func (r *Rule) validate() error {
	c, t, i := r.ComplaintCluster, r.TestFailure, r.IllnessCluster
	switch {
	case r.Version == "":
		return errors.New("rule: version required")
	case len(c.IssueTypes) == 0:
		return errors.New("rule: complaint_cluster.issue_types required")
	case c.WindowDays <= 0 || c.MinComplaints <= 0 || c.MinDistinctReporters <= 0 || c.PairwiseRadiusM <= 0 || c.StaleOpenDays <= 0:
		return errors.New("rule: complaint_cluster values must be positive")
	case c.MinDistinctReporters > c.MinComplaints:
		return errors.New("rule: min_distinct_reporters cannot exceed min_complaints")
	case t.WindowDays <= 0:
		return errors.New("rule: test_failure.window_days must be positive")
	case i.WindowHours <= 0 || i.MinCases <= 0:
		return errors.New("rule: illness_cluster values must be positive")
	case i.CasesPerReport != "max" && i.CasesPerReport != "sum":
		return errors.New("rule: illness_cluster.cases_per_report must be max or sum")
	case i.ORS.Multiplier <= 1 || i.ORS.BaselineDays <= 0 || i.ORS.MinBaselineReports <= 0 || i.ORS.WindowHours <= 0:
		return errors.New("rule: illness_cluster.ors values invalid")
	case r.Levels.CriticalMinIllnessCases <= 0:
		return errors.New("rule: levels.critical_min_illness_cases must be positive")
	}
	for _, it := range c.IssueTypes {
		if !domain.Contains(domain.IssueTypes, it) {
			return fmt.Errorf("rule: unknown issue type %q", it)
		}
	}
	return nil
}

// LookbackDays is how much history Evaluate needs; callers load at least this.
func (r *Rule) LookbackDays() int {
	d := r.ComplaintCluster.WindowDays
	d = max(d, r.TestFailure.WindowDays)
	d = max(d, r.IllnessCluster.ORS.BaselineDays+(r.IllnessCluster.ORS.WindowHours+23)/24+1)
	return d + 1
}
