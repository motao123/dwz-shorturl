package pkg

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
)

// ViolationStatus is the review state of a shortened URL.
type ViolationStatus string

const (
	ViolationPending ViolationStatus = "pending" // detection not yet run
	ViolationPassed  ViolationStatus = "passed"  // looks safe
	ViolationReview  ViolationStatus = "review"  // suspicious, needs human review
	ViolationBlocked ViolationStatus = "blocked" // clearly violating, do not serve
)

// ViolationResult carries the outcome of a synchronous violation check.
type ViolationResult struct {
	Status ViolationStatus
	Reason string
}

// violationRulesJSON is the shared rule set, also read by the PHP frontend.
// Embedding it makes the file part of the binary, so a deployment cannot end up
// with Go and PHP disagreeing about what is blocked.
//
//go:embed data/violation_rules.json
var violationRulesJSON []byte

// violationRules mirrors testsrc/violation_rules.json.
type violationRules struct {
	DomainSuffixes []string `json:"domain_suffixes"`
	Keywords       []string `json:"keywords"`
}

var (
	rulesOnce             sync.Once
	blockedDomainSuffixes []string
	blockedKeywords       []string
)

// loadViolationRules parses the embedded rule set once. A parse failure is
// panic-worthy: shipping a build whose compliance rules silently vanished would
// be far worse than failing to start.
func loadViolationRules() {
	rulesOnce.Do(func() {
		var r violationRules
		if err := json.Unmarshal(violationRulesJSON, &r); err != nil {
			panic("pkg: cannot parse embedded violation_rules.json: " + err.Error())
		}
		blockedDomainSuffixes = r.DomainSuffixes
		blockedKeywords = r.Keywords
	})
}

// BlockedDomainSuffixes returns the shared domain blacklist (exact or suffix
// match). Exported so tests and ops tooling can assert both stacks agree.
func BlockedDomainSuffixes() []string {
	loadViolationRules()
	return blockedDomainSuffixes
}

// BlockedKeywords returns the shared keyword blacklist (case-insensitive
// substring match on the full URL).
func BlockedKeywords() []string {
	loadViolationRules()
	return blockedKeywords
}

// CheckURLViolation performs a synchronous, rule-based violation check on a
// destination URL. It NEVER performs network fetches (the URL is not resolved
// here), so it is safe and fast. Detection returns one of:
//   - ViolationBlocked: clearly violating, the link must be rejected.
//   - ViolationReview:  suspicious pattern, flag for human review.
//   - ViolationPassed:  no obvious violation found.
func CheckURLViolation(rawURL string) ViolationResult {
	loadViolationRules()
	u, err := url.Parse(rawURL)
	if err != nil {
		return ViolationResult{Status: ViolationBlocked, Reason: "url is invalid"}
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	lower := strings.ToLower(rawURL)

	// Cloud metadata endpoints are always blocked (SSRF defence-in-depth).
	if host == "169.254.169.254" ||
		strings.HasSuffix(host, ".169.254.169.254") ||
		host == "metadata.google.internal" ||
		strings.HasSuffix(host, "metadata.google.internal") {
		return ViolationResult{Status: ViolationBlocked, Reason: "cloud metadata address is not allowed"}
	}

	// TODO: fetch the page and keyword-scan its body (async deep detection).
	// The synchronous check below only inspects the URL itself.
	_ = lower

	// Exact / suffix domain blacklist.
	if host != "" {
		for _, d := range blockedDomainSuffixes {
			if host == d || strings.HasSuffix(host, "."+d) {
				return ViolationResult{Status: ViolationBlocked, Reason: "domain is blocked"}
			}
		}
	}

	// Substring keyword rules on the full URL.
	for _, kw := range blockedKeywords {
		if strings.Contains(lower, kw) {
			return ViolationResult{Status: ViolationBlocked, Reason: "url matches a blocked keyword"}
		}
	}

	return ViolationResult{Status: ViolationPassed, Reason: ""}
}
