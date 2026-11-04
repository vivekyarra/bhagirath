package rule

// FrozenV1SHA256 is the SHA-256 of config/alert_rule_v1.yaml at the git tag
// rule-v1-frozen. The server refuses to run a "v1" rule whose bytes differ,
// and CI fails if the file is edited (spec Section 8: never edit v1).
const FrozenV1SHA256 = "9366e3fa9535b3ab58ef02f64dd1cb8a46ebb58d2d81a8e29ea80729271b5e93"

// FrozenTag is the git tag that proves when the rule was frozen.
const FrozenTag = "rule-v1-frozen"

// CheckFrozen returns an error if r claims to be v1 but is not the frozen file.
func (r *Rule) CheckFrozen() error {
	if r.Version == "v1" && r.SHA256 != FrozenV1SHA256 {
		return errFrozen
	}
	return nil
}

type frozenErr struct{}

func (frozenErr) Error() string {
	return "config/alert_rule_v1.yaml differs from the frozen v1 rule; create alert_rule_v2.yaml instead of editing v1"
}

var errFrozen error = frozenErr{}
