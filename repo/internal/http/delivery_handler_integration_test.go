package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// delivery_handler_integration_test.go exercises the full dispatch
// pipeline via the HTTP layer: facility lookup, eligibility filtering,
// strategy selection, and audit write.

// morningPickup returns a pickup time at 09:00 UTC a few days in the
// future. Using a fixed hour (well before the 20:00 facility cutoff)
// keeps the tests deterministic regardless of the wall-clock hour the
// suite is executed in.
func morningPickup(offset time.Duration) time.Time {
	base := time.Date(2099, 1, 15, 9, 0, 0, 0, time.UTC)
	return base.Add(offset)
}

func TestAssignCourierHappyPath(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	id := seedDelivery(t, h, "east", morningPickup(0))
	body := `{"strategy":"rating-first","facility_id":"fac-main"}`
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["courier_id"] == nil {
		t.Fatal("expected courier_id in response")
	}
	if got["strategy"] != "rating-first" {
		t.Fatalf("expected strategy echo, got %v", got["strategy"])
	}
}

func TestAssignCourierBlacklistedZoneSurfacesConflict(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	id := seedDelivery(t, h, "restricted-north", morningPickup(0))
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", `{"facility_id":"fac-main"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 zone blacklisted, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["conflict"] != true {
		t.Fatal("expected conflict=true flag in response")
	}
}

func TestAssignCourierAfterCutoffSurfacesConflict(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	evening := time.Date(2099, 3, 28, 22, 0, 0, 0, time.UTC)
	id := seedDelivery(t, h, "east", evening)
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", `{"facility_id":"fac-main"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 after cutoff, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDeliveriesEnumerates(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	// 2-hour spacing so the ids (formatted from time HHMMSS) are unique.
	_ = seedDelivery(t, h, "east", morningPickup(0))
	_ = seedDelivery(t, h, "west", morningPickup(2*time.Hour))
	rec := h.do(t, http.MethodGet, "/api/deliveries", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	ds, _ := body["deliveries"].([]any)
	if len(ds) < 2 {
		t.Fatalf("expected >=2 deliveries, got %d body=%s", len(ds), rec.Body.String())
	}
}

// seedDelivery directly persists a delivery order so the test can skip
// the create endpoint and focus on assignment behaviour.
func seedDelivery(t *testing.T, h *harness, zone string, pickup time.Time) string {
	t.Helper()
	return seedDeliveryInOrg(t, h, zone, pickup, "org-main")
}

// seedDeliveryInOrg persists a delivery under an explicit organisation,
// used by cross-tenant negative tests.
func seedDeliveryInOrg(t *testing.T, h *harness, zone string, pickup time.Time, orgID string) string {
	t.Helper()
	o := models.Order{
		ID:         "dlv-" + orgID + "-" + zone + "-" + pickup.Format("20060102150405"),
		Number:     "HC-TEST-" + orgID + "-" + zone + "-" + pickup.Format("150405"),
		Kind:       models.OrderDelivery,
		State:      models.StateCreated,
		OrgID:      orgID,
		PickupZone: zone,
		PickupAt:   pickup,
		CreatedAt:  time.Now(),
	}
	if err := h.store.CreateOrder(context.Background(), o); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	return o.ID
}

// TestListDeliveriesIsOrgScoped seeds deliveries for two organisations
// and verifies the dispatcher of one org never sees the other org's
// queue.
func TestListDeliveriesIsOrgScoped(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	_ = seedDeliveryInOrg(t, h, "east", morningPickup(0), "org-main")
	foreign := seedDeliveryInOrg(t, h, "east", morningPickup(3*time.Hour), "org-other")
	rec := h.do(t, http.MethodGet, "/api/deliveries", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	ds, _ := body["deliveries"].([]any)
	for _, d := range ds {
		if m, ok := d.(map[string]any); ok {
			if id, _ := m["ID"].(string); id == foreign {
				t.Fatalf("dispatcher saw foreign-org delivery %s", id)
			}
		}
	}
}

// TestAssignCourierCrossOrgForbidden proves that an attempt to assign
// a courier to a delivery belonging to a different organisation is
// rejected with 403, regardless of the role gate passing.
func TestAssignCourierCrossOrgForbidden(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	id := seedDeliveryInOrg(t, h, "east", morningPickup(0), "org-other")
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", `{"facility_id":"fac-main"}`, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestListDeliveriesForbiddenForStudent confirms non-dispatcher roles
// are denied the delivery queue entirely.
func TestListDeliveriesForbiddenForStudent(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/deliveries", "", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for student, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestDispatcherPageOrgScoped proves the /dispatcher server-rendered
// view is scoped to the dispatcher's organisation and never leaks
// foreign-org delivery numbers into the HTML.
func TestDispatcherPageOrgScoped(t *testing.T) {
	h := newHarness(t)
	_ = seedDeliveryInOrg(t, h, "east", morningPickup(0), "org-main")
	foreign := seedDeliveryInOrg(t, h, "east", morningPickup(3*time.Hour), "org-other")
	foreignOrder, err := h.store.OrderByID(context.Background(), foreign)
	if err != nil {
		t.Fatal(err)
	}

	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	rec := h.do(t, http.MethodGet, "/dispatcher", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), foreignOrder.Number) {
		t.Fatalf("dispatcher page leaked foreign-org order number %s", foreignOrder.Number)
	}
}

// TestCompleteDeliveryAllowsCourier proves the assigned courier can
// mark a delivery completed through the new HTTP lifecycle endpoint.
func TestCompleteDeliveryAllowsCourier(t *testing.T) {
	h := newHarness(t)
	id := seedDelivery(t, h, "east", morningPickup(0))
	ctx := context.Background()
	o, err := h.store.OrderByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	o.CourierID = "usr-courier"
	o.State = models.StateInProgress
	if err := h.store.UpdateOrder(ctx, o); err != nil {
		t.Fatal(err)
	}
	tok := h.loginAs(t, "courier", "courier-pass")
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/complete", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	if body["state"] != string(models.StateCompleted) {
		t.Fatalf("expected completed, got %v", body["state"])
	}
}
