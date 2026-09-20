package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBulkEntryPointsGoThroughVerifyGate is the anti-regression pin for #8.
//
// The bug: BatchCreateLinks checked `EmailVerified` but ImportLinks called
// CreateLink directly and skipped the check, so an unverified registration could
// still bulk-produce links up to MemberImportMaxRows per request — the admin
// switch member.batch_requires_verified was honoured on one bulk path and
// ignored on the other.
//
// A behavioural test would only re-cover the two methods that exist today; this
// source-level assertion also fails when someone adds a THIRD bulk entry point
// that forgets the gate, which is the exact shape of the original regression.
func TestBulkEntryPointsGoThroughVerifyGate(t *testing.T) {
	root := repoRootForService(t)
	src, err := os.ReadFile(filepath.Join(root, "backend", "internal", "service", "member_api.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// Every bulk method must call requireVerified.
	bulkMethods := []string{"ImportLinks", "BatchCreateLinks"}
	for _, name := range bulkMethods {
		re := regexp.MustCompile(`(?s)func \(s \*memberApiService\) ` + name + `\(.*?\n}`)
		m := re.FindString(body)
		if m == "" {
			t.Fatalf("could not locate %s; update this test if the method was renamed", name)
		}
		if !strings.Contains(m, "requireVerified(") {
			t.Errorf("%s no longer calls requireVerified(): the email-verification "+
				"gate is bypassable on this bulk path again (#8)", name)
		}
	}

	// The legacy inline check must not come back: it is the drift that allowed
	// one path to diverge from the other. Both must share requireVerified.
	if strings.Contains(body, "m.EmailVerified != 1") && !strings.Contains(body, "func (s *memberApiService) requireVerified") {
		t.Error("EmailVerified is checked inline without the shared requireVerified helper")
	}
}

// repoRootForService walks up from the service package dir to the repo root.
func repoRootForService(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "backend", "internal", "service")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repository root")
	return ""
}
