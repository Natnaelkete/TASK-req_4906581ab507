package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
)

// authHash mints a password hash using the same salt bootstrap.Seed
// uses, so a test-seeded user can log in through the real auth path.
func authHash(pw string) string {
	return auth.HashPassword(pw, "harborclass-demo")
}

// booking_handler_integration_test.go drives the real Gin router end
// to end: real handlers, real state-machine, real audit chain, real
// in-memory persistence. No mocks.

func TestListSessionsReturnsSeededSession(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/sessions", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	sessions, ok := body["sessions"].([]any)
	if !ok || len(sessions) == 0 {
		t.Fatalf("expected at least one seeded session, body=%s", rec.Body.String())
	}
}

// TestListSessionsRequiresAuth locks in that the session catalog is
// no longer a public surface.
func TestListSessionsRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/sessions", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

// TestListSessionsHidesForeignOrg proves cross-org session metadata
// is not enumerated through the catalog API.
func TestListSessionsHidesForeignOrg(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_ = h.store.CreateSession(ctx, models.Session{
		ID: "sess-scan", TeacherID: "usr-teacher", ClassID: "class-default",
		OrgID: "org-other", Title: "Other org",
		StartsAt: time.Now().Add(48 * time.Hour), EndsAt: time.Now().Add(49 * time.Hour),
		Capacity: 5,
	})
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/sessions", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	sessions, _ := body["sessions"].([]any)
	for _, s := range sessions {
		if m, ok := s.(map[string]any); ok {
			if id, _ := m["ID"].(string); id == "sess-scan" {
				t.Fatalf("foreign-org session leaked into catalog")
			}
		}
	}
}

func TestCreateBookingHappyPath(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	number, _ := body["number"].(string)
	if !strings.HasPrefix(number, "HC-") {
		t.Fatalf("expected HC- prefixed number, got %v", body["number"])
	}
	if body["state"] != string(models.StateConfirmed) {
		t.Fatalf("expected state confirmed, got %v", body["state"])
	}
}

func TestCreateBookingRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRescheduleEnforcesCap(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	for i := 0; i < 2; i++ {
		rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/reschedule", `{"new_start":"2026-05-01T10:00:00Z"}`, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	// third reschedule must be rejected.
	rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/reschedule", `{"new_start":"2026-06-01T10:00:00Z"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCancellationInside24hNeedsApproval(t *testing.T) {
	h := newHarness(t)
	// Inject a near-future session so the default 48h seeded one doesn't apply.
	ctx := context.Background()
	_ = h.store.CreateSession(ctx, models.Session{
		ID: "sess-soon", TeacherID: "usr-teacher", ClassID: "class-default",
		OrgID: "org-main",
		Title: "Tomorrow", StartsAt: time.Now().Add(6 * time.Hour), EndsAt: time.Now().Add(7 * time.Hour),
		Capacity: 5,
	})
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-soon"}`, tok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)
	rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/cancel", "", tok)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 awaiting approval, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetBookingReturnsTimeline(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["timeline"] == nil {
		t.Fatal("expected timeline in response")
	}
}

func TestSubscriptionUnsubscribeIsOneClick(t *testing.T) {
	h := newHarness(t)
	// One-click URL is auth-less but signed with an HMAC token that
	// binds the user+category pair and expires after 30 days.
	tok := h.signUnsubscribe("usr-student", "booking")
	rec := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking&token="+tok, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	sub, _ := h.store.Subscription(context.Background(), "usr-student", "booking")
	if sub.Subscribed {
		t.Fatal("expected subscription to be off after one-click unsubscribe")
	}
}

// TestUnsubscribeRejectsUnsignedRequest proves unauthenticated callers
// cannot toggle arbitrary users' subscriptions without a valid token.
func TestUnsubscribeRejectsUnsignedRequest(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking", "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 unsigned, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestUnsubscribeRejectsForgedToken proves the HMAC signature is bound
// to the specific user+category; a token for a different pair fails.
func TestUnsubscribeRejectsForgedToken(t *testing.T) {
	h := newHarness(t)
	tok := h.signUnsubscribe("usr-teacher", "booking")
	rec := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking&token="+tok, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 forged, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetBookingDeniesForeignStudent proves students cannot read each
// other's bookings — only the owning student's token passes.
func TestGetBookingDeniesForeignStudent(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, ownerTok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	// Seed a second student in the same org and log in as them.
	if err := h.store.CreateUser(context.Background(), models.User{
		ID: "usr-other-student", Username: "other-student", Role: models.RoleStudent,
		OrgID: "org-main", ClassIDs: []string{"class-default"},
		PasswordHash: authHash("other-pass"),
	}); err != nil {
		t.Fatal(err)
	}
	otherTok := h.loginAs(t, "other-student", "other-pass")
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", otherTok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign student, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetBookingDeniesCrossOrgAdmin proves an authenticated admin from a
// different organisation cannot read bookings outside their tenant.
func TestGetBookingDeniesCrossOrgAdmin(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, ownerTok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	// Seed an admin in a different org.
	if err := h.store.CreateUser(context.Background(), models.User{
		ID: "usr-foreign-admin", Username: "foreign-admin", Role: models.RoleAdmin,
		OrgID: "org-other", PasswordHash: authHash("foreign-pass"),
	}); err != nil {
		t.Fatal(err)
	}
	foreignTok := h.loginAs(t, "foreign-admin", "foreign-pass")
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", foreignTok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org admin, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetBookingDeniesDispatcher proves dispatchers are not implicitly
// allowed to read booking orders through the GetBooking endpoint; they
// have their own delivery-scoped surface.
func TestGetBookingDeniesDispatcher(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, ownerTok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	dispTok := h.loginAs(t, "dispatcher", "dispatch-pass")
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", dispTok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for dispatcher, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetBookingAllowsSameOrgTeacher proves a teacher in the booking's
// organisation can read it; scope is same-org, not ownership.
func TestGetBookingAllowsSameOrgTeacher(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, ownerTok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	teacherTok := h.loginAs(t, "teacher", "teacher-pass")
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", teacherTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 same-org teacher, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCompleteThenRefundOpensSevenDayWindow drives the documented
// completion -> refund-request happy path end-to-end through HTTP so
// the lifecycle is demonstrably reachable from the public API.
func TestCompleteThenRefundOpensSevenDayWindow(t *testing.T) {
	h := newHarness(t)
	studentTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, studentTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create booking: %d %s", create.Code, create.Body.String())
	}
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	teacherTok := h.loginAs(t, "teacher", "teacher-pass")
	comp := h.do(t, http.MethodPost, "/api/bookings/"+id+"/complete", "", teacherTok)
	if comp.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", comp.Code, comp.Body.String())
	}
	got := mustJSON(t, comp.Body.Bytes())
	if got["state"] != string(models.StateCompleted) {
		t.Fatalf("expected completed state, got %v", got["state"])
	}

	refund := h.do(t, http.MethodPost, "/api/bookings/"+id+"/refund-request", "", studentTok)
	if refund.Code != http.StatusOK {
		t.Fatalf("refund request: %d %s", refund.Code, refund.Body.String())
	}
	rbody := mustJSON(t, refund.Body.Bytes())
	if rbody["state"] != string(models.StateRefundReview) {
		t.Fatalf("expected refund_review, got %v", rbody["state"])
	}
}

// TestCompleteBookingDeniesForeignTeacher proves only the owning
// teacher (or a same-org admin) can mark completion; cross-org or
// unrelated teachers are rejected.
func TestCompleteBookingDeniesForeignTeacher(t *testing.T) {
	h := newHarness(t)
	studentTok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, studentTok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	if err := h.store.CreateUser(context.Background(), models.User{
		ID: "usr-foreign-teacher-2", Username: "foreign-teacher-2", Role: models.RoleTeacher,
		OrgID: "org-other", PasswordHash: authHash("foreign-pass"),
	}); err != nil {
		t.Fatal(err)
	}
	foreignTok := h.loginAs(t, "foreign-teacher-2", "foreign-pass")
	rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/complete", "", foreignTok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org teacher, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCreateBookingRejectsForeignOrgSession proves a student cannot
// book a session belonging to another organisation.
func TestCreateBookingRejectsForeignOrgSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_ = h.store.CreateSession(ctx, models.Session{
		ID: "sess-foreign", TeacherID: "usr-teacher", ClassID: "class-default",
		OrgID: "org-other",
		Title: "Foreign", StartsAt: time.Now().Add(48 * time.Hour), EndsAt: time.Now().Add(49 * time.Hour),
		Capacity: 5,
	})
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-foreign"}`, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org session, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTeacherAnalyticsFormat(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	rec := h.do(t, http.MethodGet, "/api/teacher/analytics", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, key := range []string{"window_7d", "window_30d", "window_90d"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing %s in analytics payload: %s", key, rec.Body.String())
		}
	}
}
