package webtpl_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

func TestDispatcherDashboardShowsStrategyAndDeliveries(t *testing.T) {
	d := webtpl.DispatcherData{
		User: models.User{DisplayName: "Dana Dispatcher", Role: models.RoleDispatcher},
		Couriers: []models.User{
			{ID: "c1", DisplayName: "Casey Courier", Rating: 4.9, Load: 2},
		},
		Deliveries: []models.Order{
			{ID: "dlv-east-01", Number: "HC-03282026-000011", PickupZone: "east", PickupAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC), State: models.StateInProgress},
		},
	}
	html := webtpl.RenderDispatcherDashboard(d)
	for _, need := range []string{
		`data-role="dispatcher"`,
		"Dispatcher Console",
		"Dana Dispatcher",
		"distance-first",
		"rating-first",
		"load-balanced",
		"HC-03282026-000011",
		"Casey Courier",
		"east",
		"state-badge state-in_progress",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("dispatcher HTML missing %q", need)
		}
	}
}

// TestDispatcherAssignFormIsPerOrder verifies the UI action path is a
// parameterised /api/deliveries/{id}/assign and not the bare endpoint,
// which matches the backend route contract in router.go.
func TestDispatcherAssignFormIsPerOrder(t *testing.T) {
	d := webtpl.DispatcherData{
		User: models.User{DisplayName: "Dana Dispatcher", Role: models.RoleDispatcher},
		Deliveries: []models.Order{
			{ID: "dlv-east-01", Number: "HC-03282026-000011", PickupZone: "east", PickupAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC), State: models.StateInProgress},
		},
	}
	html := webtpl.RenderDispatcherDashboard(d)
	need := `action="/api/deliveries/dlv-east-01/assign"`
	if !strings.Contains(html, need) {
		t.Fatalf("dispatcher HTML missing per-order form action %q\nhtml=%s", need, html)
	}
	if strings.Contains(html, `action="/api/deliveries/assign"`) {
		t.Fatal("dispatcher HTML still renders unparameterised /api/deliveries/assign action")
	}
}

func TestDispatcherDashboardRendersConflictMessages(t *testing.T) {
	d := webtpl.DispatcherData{
		User:          models.User{DisplayName: "Dana", Role: models.RoleDispatcher},
		ConflictNotes: []string{"pickup falls after facility cutoff", "courier cannot service the requested zone"},
	}
	html := webtpl.RenderDispatcherDashboard(d)
	for _, need := range []string{
		"Conflicts",
		"pickup falls after facility cutoff",
		"courier cannot service the requested zone",
		"conflict-msg",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("conflicts HTML missing %q", need)
		}
	}
}

func TestDispatcherEmptyConflictsIsClean(t *testing.T) {
	d := webtpl.DispatcherData{User: models.User{DisplayName: "Dana", Role: models.RoleDispatcher}}
	html := webtpl.RenderDispatcherDashboard(d)
	if !strings.Contains(html, "No conflicts detected") {
		t.Fatalf("expected empty-conflicts placeholder, got %s", html)
	}
}
