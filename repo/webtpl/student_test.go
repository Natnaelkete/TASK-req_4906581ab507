package webtpl_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

// student_test.go is a frontend unit test: it renders the Templ-compiled
// student dashboard and asserts required markers are present so the
// role-aware UI remains static-auditable.

func TestStudentDashboardRendersTimelineAndBadge(t *testing.T) {
	at := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	data := webtpl.StudentData{
		User: models.User{DisplayName: "Alex Student", Role: models.RoleStudent},
		Orders: []models.Order{
			{
				ID: "ord-1", Number: "HC-03282026-000001", State: models.StateConfirmed, Payment: models.PayUnpaid,
				Timeline: []models.OrderEvent{
					{At: at, State: models.StateCreated, Message: "order created"},
					{At: at.Add(1 * time.Minute), State: models.StateConfirmed, Message: "confirmed"},
				},
			},
		},
	}
	html := webtpl.RenderStudentDashboard(data)
	for _, need := range []string{
		`data-role="student"`,
		"Student Dashboard",
		"Alex Student",
		"HC-03282026-000001",
		"state-badge state-confirmed",
		"Confirmed",
		"order created",
		"confirmed",
		"Cancel",
		"Request reschedule",
		"Request refund",
		"Message preferences",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("student HTML missing %q\nhtml=%s", need, html)
		}
	}
}

func TestStudentDashboardEmptyState(t *testing.T) {
	html := webtpl.RenderStudentDashboard(webtpl.StudentData{
		User: models.User{DisplayName: "Alex Student", Role: models.RoleStudent},
	})
	if !strings.Contains(html, "No orders yet") {
		t.Fatalf("expected empty-state copy, got %s", html)
	}
}
