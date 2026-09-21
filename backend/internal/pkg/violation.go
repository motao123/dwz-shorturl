package pkg

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
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
// Embedding keeps a build self-contained: a deployment that never shipped the
// file still blocks the same things Go was compiled with.
//
//go:embed data/violation_rules.json
var violationRulesJSON []byte

// Runtime rule sources (#33). Embedding alone caused *staleness* drift: PHP
// re-reads the file on every request, so an emergency blocklist edit took effect
// on the public entry points immediately while Go (admin console, /public/api,
// /r/:code) kept serving with the compiled-in copy until someone rebuilt.
// Both stacks now look at the same operator-editable path, so a single edit
// moves both. The embedded set stays as the fallback, never as a silent override.
const (
	ViolationRulesEnv  = "DWZ_VIOLATION_RULES_FILE"
	ViolationRulesPath = "/etc/dwz/violation_rules.json"
)

// rulesCheckInterval throttles the stat() on the hot path: the check runs on
// every create/redirect, and a syscall per call buys nothing over a few seconds.
// A var so tests can force a reload deterministically.
var rulesCheckInterval = 5 * time.Second

// violationRules mirrors testsrc/violation_rules.json.
type violationRules struct {
	DomainSuffixes []string `json:"domain_suffixes"`
	Keywords       []string `json:"keywords"`
}

// violationRuleSet is one immutable snapshot plus where it came from, so readers
// never observe a half-swapped pair of lists.
type violationRuleSet struct {
	suffixes []string
	keywords []string
	source   string
}

var (
	rulesOnce      sync.Once
	rulesMu        sync.Mutex
	rulesCur       *violationRuleSet
	rulesMTime     time.Time
	rulesCheckedAt time.Time
	rulesErr       error
)

func parseViolationRules(raw []byte) (*violationRules, error) {
	var r violationRules
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// loadViolationRules resolves the initial set. A build whose embedded rules cannot
// be parsed is panic-worthy: shipping a binary whose compliance rules silently
// vanished is worse than refusing to start.
func loadViolationRules() {
	rulesOnce.Do(func() {
		r, err := parseViolationRules(violationRulesJSON)
		if err != nil {
			panic("pkg: cannot parse embedded violation_rules.json: " + err.Error())
		}
		rulesCur = &violationRuleSet{
			suffixes: r.DomainSuffixes,
			keywords: r.Keywords,
			source:   "embedded",
		}
		if p := violationRulesFile(); p != "" {
			if set, err := readViolationRulesFile(p); err == nil {
				rulesCur, rulesErr = set, nil
			} else {
				rulesErr = err
			}
		}
	})
}

func violationRulesFile() string {
	if p := strings.TrimSpace(os.Getenv(ViolationRulesEnv)); p != "" {
		return p
	}
	if _, err := os.Stat(ViolationRulesPath); err == nil {
		return ViolationRulesPath
	}
	return ""
}

func readViolationRulesFile(path string) (*violationRuleSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := parseViolationRules(raw)
	if err != nil {
		return nil, err
	}
	return &violationRuleSet{suffixes: r.DomainSuffixes, keywords: r.Keywords, source: path}, nil
}

// currentViolationRules returns the effective snapshot, reloading when the file
// changed. A broken edit never removes coverage: the last good set keeps serving
// and the failure stays readable through ViolationRulesError().
func currentViolationRules() *violationRuleSet {
	loadViolationRules()

	rulesMu.Lock()
	defer rulesMu.Unlock()
	path := violationRulesFile()
	if path == "" {
		return rulesCur
	}
	now := time.Now()
	if !rulesCheckedAt.IsZero() && now.Sub(rulesCheckedAt) < rulesCheckInterval {
		return rulesCur
	}
	rulesCheckedAt = now
	mt, err := os.Stat(path)
	if err != nil {
		rulesErr = err
		return rulesCur
	}
	if !rulesMTime.Equal(mt.ModTime()) {
		if set, readErr := readViolationRulesFile(path); readErr != nil {
			rulesErr = readErr
		} else {
			rulesCur, rulesMTime, rulesErr = set, mt.ModTime(), nil
		}
	}
	return rulesCur
}

// ViolationRulesSource reports where the effective rules came from, for ops
// endpoints and tests ("embedded" or a file path).
func ViolationRulesSource() string { return currentViolationRules().source }

// ViolationRulesError reports the most recent rules-file problem (unreadable or
// unparseable). The process keeps running on the last good set, so without this
// accessor a bad edit would be invisible.
func ViolationRulesError() error {
	currentViolationRules()
	rulesMu.Lock()
	defer rulesMu.Unlock()
	return rulesErr
}

// BlockedDomainSuffixes returns the shared domain blacklist (exact or suffix
// match). Exported so tests and ops tooling can assert both stacks agree.
func BlockedDomainSuffixes() []string {
	return currentViolationRules().suffixes
}

// BlockedKeywords returns the shared keyword blacklist (case-insensitive
// substring match on the full URL).
func BlockedKeywords() []string {
	return currentViolationRules().keywords
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

	// Exact / suffix domain blacklist. One snapshot for the whole check, so a
	// concurrent hot reload can never make this URL's verdict mix two rule sets.
	rules := currentViolationRules()
	if host != "" {
		for _, d := range rules.suffixes {
			if host == d || strings.HasSuffix(host, "."+d) {
				return ViolationResult{Status: ViolationBlocked, Reason: "domain is blocked"}
			}
		}
	}

	// Substring keyword rules on the full URL.
	for _, kw := range rules.keywords {
		if strings.Contains(lower, kw) {
			return ViolationResult{Status: ViolationBlocked, Reason: "url matches a blocked keyword"}
		}
	}

	return ViolationResult{Status: ViolationPassed, Reason: ""}
}
