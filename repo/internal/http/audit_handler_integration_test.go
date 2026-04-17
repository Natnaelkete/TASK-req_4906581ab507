package http_test

import (
	"net/http"
	"strings"
	"testing"
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
