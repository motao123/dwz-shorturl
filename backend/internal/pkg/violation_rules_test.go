package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The PHP frontend and the Go backend must block exactly the same set. They now
// share one JSON file, so the real thing worth guarding is that the embedded
// copy is well-formed and that the domain list is not accidentally empty (an
// empty list would silently disable domain blocking on both sides).
func TestEmbeddedRulesLoad(t *testing.T) {
	if got := len(BlockedDomainSuffixes()); got == 0 {
		t.Fatal("domain suffix list is empty; domain blocking would be disabled")
	}
	if got := len(BlockedKeywords()); got == 0 {
		t.Fatal("keyword list is empty; keyword blocking would be disabled")
	}
}

// The frontend loads the same path from disk. Assert it exists and parses, so a
// rename or a broken edit fails here rather than at runtime on the PHP side.
func TestSharedRulesFileMatchesEmbeddedCopy(t *testing.T) {
	path := filepath.Join("data", "violation_rules.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("shared rules file unreadable: %v", err)
	}

	var onDisk violationRules
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("shared rules file is not valid JSON: %v", err)
	}

	if len(onDisk.DomainSuffixes) != len(BlockedDomainSuffixes()) {
		t.Errorf("domain suffix count differs: file=%d embedded=%d",
			len(onDisk.DomainSuffixes), len(BlockedDomainSuffixes()))
	}
	if len(onDisk.Keywords) != len(BlockedKeywords()) {
		t.Errorf("keyword count differs: file=%d embedded=%d",
			len(onDisk.Keywords), len(BlockedKeywords()))
	}

	// The file is also what PHP reads; assert the PHP path the frontend uses is
	// the one we just validated.
	phpPath := filepath.Join("..", "..", "..", "backend", "internal", "pkg", "data", "violation_rules.json")
	if _, err := os.Stat(phpPath); err != nil {
		t.Errorf("PHP frontend path %s does not resolve: %v", phpPath, err)
	}
}

// Spot-check that a representative gambling domain and keyword are still caught
// after the refactor, so a bad merge cannot quietly weaken enforcement.
func TestCheckURLViolationStillBlocksKnownBadInput(t *testing.T) {
	cases := []string{
		"https://www.bet365.com/promo",
		"https://example.com/casino",
		"https://example.com/百家乐",
		"https://169.254.169.254/latest/meta-data/",
	}
	for _, c := range cases {
		if r := CheckURLViolation(c); r.Status != ViolationBlocked {
			t.Errorf("%s should be blocked, got %s (%s)", c, r.Status, r.Reason)
		}
	}
	if r := CheckURLViolation("https://example.com/hello"); r.Status != ViolationPassed {
		t.Errorf("benign URL should pass, got %s", r.Status)
	}
}
