package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/eaglepoint/harborclass/internal/models"
)

// admin_handler_integration_test.go covers object-level authorisation
// and the configurable-permissions overlay via the admin surface.

// TestAdminMembershipPersistsClassIDs verifies that adjusting membership
// writes through to the store under its Postgres-style UpdateUser path.
func TestAdminMembershipPersistsClassIDs(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "admin", "admin-pass")
	body := `{"username":"teacher","class_ids":["class-default","class-advanced"]}`
	rec := h.do(t, http.MethodPost, "/api/admin/membership", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	u, err := h.store.UserByUsername(context.Background(), "teacher")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	want := map[string]bool{"class-default": true, "class-advanced": true}
	if len(u.ClassIDs) != len(want) {
		t.Fatalf("want %d class ids, got %v", len(want), u.ClassIDs)
	}
	for _, c := range u.ClassIDs {
		if !want[c] {
			t.Fatalf("unexpected class id %q", c)
		}
	}
}

// TestAdminMembershipRejectsCrossOrg asserts that an admin cannot
// modify a user in another organisation.
func TestAdminMembershipRejectsCrossOrg(t *testing.T) {
	h := newHarness(t)
	// Seed a user in a different org.
	if err := h.store.CreateUser(context.Background(), models.User{
		ID: "usr-foreign", Username: "foreign", Role: models.RoleTeacher,
		OrgID: "org-other", PasswordHash: "sha256$s$x",
	}); err != nil {
		t.Fatal(err)
	}
	tok := h.loginAs(t, "admin", "admin-pass")
	body := `{"username":"foreign","class_ids":["class-default"]}`
	rec := h.do(t, http.MethodPost, "/api/admin/membership", body, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminPermissionsOverlayEnforcedOnNextAction proves that a stored
// permission grant (teacher -> export_audit) lets the teacher export
// the audit log on the very next request, and that the overlay stays
// org-scoped. The handler uses h.can, so the overlay is authoritative.
func TestAdminPermissionsOverlayEnforcedOnNextAction(t *testing.T) {
	h := newHarness(t)
	adminTok := h.loginAs(t, "admin", "admin-pass")
	teacherTok := h.loginAs(t, "teacher", "teacher-pass")

	// Baseline: teacher is denied the audit export.
	base := h.do(t, http.MethodGet, "/api/audit-logs/export", "", teacherTok)
	if base.Code != http.StatusForbidden {
		t.Fatalf("baseline expected 403, got %d", base.Code)
	}

	// Admin grants export_audit to teachers in its own org.
	body := `{"permissions":[{"action":"export_audit","roles":["teacher","admin"]}]}`
	up := h.do(t, http.MethodPost, "/api/admin/permissions", body, adminTok)
	if up.Code != http.StatusOK {
		t.Fatalf("expected permission upsert 200, got %d body=%s", up.Code, up.Body.String())
	}

	// Overlay row was persisted and is loadable.
	perms, err := h.store.ListPermissions(context.Background(), "org-main")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range perms {
		if p.Action == "export_audit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("export_audit permission missing from overlay: %+v", perms)
	}

	// After the overlay the teacher can export within the same org.
	after := h.do(t, http.MethodGet, "/api/audit-logs/export", "", teacherTok)
	if after.Code != http.StatusOK {
		t.Fatalf("overlay expected 200 for teacher export, got %d body=%s", after.Code, after.Body.String())
	}
}

// TestAdminMembershipRejectsInvalidRole proves that role strings
// outside the canonical enum are rejected with 400 so authorisation
// behaviour cannot drift via arbitrary writes.
func TestAdminMembershipRejectsInvalidRole(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "admin", "admin-pass")
	body := `{"username":"teacher","class_ids":["class-default"],"role":"god-mode"}`
	rec := h.do(t, http.MethodPost, "/api/admin/membership", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid role, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminPermissionsOverlayStaysOrgScoped proves that a grant issued
// by one organisation's admin does not cross to teachers in a different
// organisation. The overlay is loaded per-subject org, so a teacher in
// org-other never sees org-main's permissions.
func TestAdminPermissionsOverlayStaysOrgScoped(t *testing.T) {
	h := newHarness(t)

	// Seed a foreign-org teacher.
	if err := h.store.CreateUser(context.Background(), models.User{
		ID: "usr-foreign-teacher", Username: "foreign-teacher", Role: models.RoleTeacher,
		OrgID: "org-other", PasswordHash: authHash("foreign-pass"),
	}); err != nil {
		t.Fatal(err)
	}

	// org-main admin grants export to teachers.
	adminTok := h.loginAs(t, "admin", "admin-pass")
	body := `{"permissions":[{"action":"export_audit","roles":["teacher","admin"]}]}`
	up := h.do(t, http.MethodPost, "/api/admin/permissions", body, adminTok)
	if up.Code != http.StatusOK {
		t.Fatalf("upsert failed: %d", up.Code)
	}

	// Teacher in a foreign org must remain denied.
	foreignTok := h.loginAs(t, "foreign-teacher", "foreign-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs/export", "", foreignTok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("overlay leaked cross-org: expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
