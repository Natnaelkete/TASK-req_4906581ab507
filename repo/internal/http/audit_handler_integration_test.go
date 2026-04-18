package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// audit_handler_integration_test.go exercises /api/audit-logs search
// and export using the real hash-chain store.

func TestSearchAuditAsAdmin(t *testing.T) {
	h := newHarness(t)
	// Drive traffic through the server to generate audit entries.
	tok := h.loginAs(t, "student", "student-pass")
	_ = h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	adminTok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs", "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	if _, ok := body["entries"]; !ok {
		t.Fatalf("missing entries key: %s", rec.Body.String())
	}
	if count, _ := body["count"].(float64); count < 1 {
		t.Fatalf("expected audit entries from prior requests, got %v", count)
	}
}

func TestSearchAuditForbiddenForNonAdmin(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs", "", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestExportAuditCSV(t *testing.T) {
	h := newHarness(t)
	_ = h.loginAs(t, "student", "student-pass") // generate at least one entry
	tok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs/export", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("expected text/csv content-type, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "id,at,actor,action,resource,detail,prev_hash,hash") {
		t.Fatalf("CSV header missing: %s", rec.Body.String())
	}
}

// TestSearchAuditIsOrgScoped proves an admin cannot read audit rows
// belonging to a different organisation. A foreign-org admin logs in
// and issues a booking.create against a session in its own org; the
// org-main admin's search must not return those rows.
func TestSearchAuditIsOrgScoped(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	// Seed a foreign org with its own session and admin+student.
	if err := h.store.CreateUser(ctx, models.User{
		ID: "usr-foreign-admin-audit", Username: "foreign-admin-audit", Role: models.RoleAdmin,
		OrgID: "org-other", PasswordHash: authHash("foreign-pass"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.CreateUser(ctx, models.User{
		ID: "usr-foreign-student-audit", Username: "foreign-student-audit", Role: models.RoleStudent,
		OrgID: "org-other", ClassIDs: []string{"class-default"},
		PasswordHash: authHash("foreign-pass"),
	}); err != nil {
		t.Fatal(err)
	}
	_ = h.store.CreateSession(ctx, models.Session{
		ID: "sess-other", TeacherID: "usr-teacher", ClassID: "class-default",
		OrgID: "org-other", Title: "Other", StartsAt: time.Now().Add(48 * time.Hour),
		EndsAt: time.Now().Add(49 * time.Hour), Capacity: 5,
	})

	// Foreign-org student creates a booking — audit row is tagged org-other.
	foreignStudentTok := h.loginAs(t, "foreign-student-audit", "foreign-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-other"}`, foreignStudentTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("foreign booking: %d %s", create.Code, create.Body.String())
	}
	foreignBody := mustJSON(t, create.Body.Bytes())
	foreignOrderID, _ := foreignBody["id"].(string)

	// org-main admin searches — must not see the foreign-org booking row.
	tok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	entries, _ := body["entries"].([]any)
	for _, e := range entries {
		m, _ := e.(map[string]any)
		resource := ""
		if v, ok := m["Resource"].(string); ok {
			resource = v
		} else if v, ok := m["resource"].(string); ok {
			resource = v
		}
		if resource == foreignOrderID {
			t.Fatalf("audit leaked foreign-org row: %+v", m)
		}
	}

	// Foreign admin sees its own booking row but not org-main's.
	foreignAdminTok := h.loginAs(t, "foreign-admin-audit", "foreign-pass")
	frec := h.do(t, http.MethodGet, "/api/audit-logs", "", foreignAdminTok)
	if frec.Code != http.StatusOK {
		t.Fatalf("foreign status %d", frec.Code)
	}
	fbody := mustJSON(t, frec.Body.Bytes())
	fentries, _ := fbody["entries"].([]any)
	foundOwn := false
	for _, e := range fentries {
		m, _ := e.(map[string]any)
		orgField := ""
		if v, ok := m["OrgID"].(string); ok {
			orgField = v
		} else if v, ok := m["org_id"].(string); ok {
			orgField = v
		}
		if orgField != "" && orgField != "org-other" {
			t.Fatalf("foreign admin saw org-main row: %+v", m)
		}
		if m["Resource"] == foreignOrderID || m["resource"] == foreignOrderID {
			foundOwn = true
		}
	}
	if !foundOwn {
		t.Fatalf("foreign admin missing own booking row in audit search: %s", frec.Body.String())
	}
}

func TestSearchAuditFilterByActor(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	_ = h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	adminTok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/audit-logs?actor=student", "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	entries, _ := body["entries"].([]any)
	if len(entries) == 0 {
		t.Fatalf("expected filtered entries for actor=student, body=%s", rec.Body.String())
	}
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		// models.AuditEntry has no JSON tags so fields serialise with Go
		// field names (capitalised). Accept either casing defensively.
		actor := m["Actor"]
		if actor == nil {
			actor = m["actor"]
		}
		if actor != "student" {
			t.Fatalf("filter leaked: entry with actor=%v returned when filtering actor=student", actor)
		}
	}
}
