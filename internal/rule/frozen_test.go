package rule

import "testing"

// TestRuleV1IsFrozen fails if anyone edits the frozen rule file.
func TestRuleV1IsFrozen(t *testing.T) {
	r := load(t)
	if err := r.CheckFrozen(); err != nil {
		t.Fatalf("%v (sha256 now %s)", err, r.SHA256)
	}
	tampered, err := Parse(append([]byte("# edit\n"), mustRead(t)...))
	if err != nil {
		t.Fatal(err)
	}
	if tampered.CheckFrozen() == nil {
		t.Fatal("edited v1 rule passed the frozen check")
	}
}
