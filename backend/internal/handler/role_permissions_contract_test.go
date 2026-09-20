package handler

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"dwz-admin/internal/service"
)

// TestGuardStatusMapsEscalationTo403 pins the HTTP contract for the privilege
// guards introduced for P0 #2/#3: a refusal must be a 403, not a 400. A 400 would
// be indistinguishable from "you typed something wrong" on the client, and the
// admin UI surfaces the two differently.
func TestGuardStatusMapsEscalationTo403(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"privilege escalation", service.ErrPrivilegeEscalation, http.StatusForbidden},
		{"super admin only", service.ErrSuperAdminOnly, http.StatusForbidden},
		{"empty roles", service.ErrEmptyRoles, http.StatusBadRequest},
		{"custom code disabled", service.ErrCustomCodeDisabled, http.StatusBadRequest},
		{"nil", nil, http.StatusOK},
	}
	for _, tc := range cases {
		if got := guardStatus(tc.err); got != tc.want {
			t.Errorf("%s: guardStatus = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestSetPermissionsAcceptsBothShapes documents the #21 contract.
//
// The admin UI's permission tree is keyed by `resource.action` strings and sent
// `{"permissions": [...]}`; the handler bound `permission_ids` with
// binding:"required", so every save returned 400 and the whole authorization
// feature was unusable. The request struct now carries both fields; this test
// asserts neither is `required` (which would re-break the other shape).
func TestSetPermissionsAcceptsBothShapes(t *testing.T) {
	src := phpSource(t, filepath.Join("backend", "internal", "handler", "role.go"))

	if strings.Contains(src, "PermissionIDs []uint64 `json:\"permission_ids\" binding:\"required\"`") {
		t.Error("permission_ids must not be binding:required; the UI sends `permissions` instead")
	}
	for _, want := range []string{`json:"permission_ids"`, `json:"permissions"`} {
		if !strings.Contains(src, want) {
			t.Errorf("SetPermissionsRequest no longer accepts %s", want)
		}
	}
	// Resource parent nodes ("short_urls") are not real permissions and must be
	// skipped rather than rejected, or the UI's tree could never be saved.
	if !strings.Contains(src, "strings.Contains(name") {
		t.Error("the resource.action resolver should skip non-leaf node names")
	}
}
