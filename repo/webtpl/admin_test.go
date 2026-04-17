package webtpl_test

import (
	"strings"
	"testing"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

func TestAdminDashboardLinksAndAlerts(t *testing.T) {
	html := webtpl.RenderAdminDashboard(webtpl.AdminData{User: models.User{DisplayName: "Avery Admin"}})
	for _, need := range []string{
		`data-role="admin"`,
		"Administrator Console",
		"Avery Admin",
		"Membership",
		"Permissions",
		"Facilities",
		"Audit",
		"Export CSV",
		"Alerts",
		"Device policies",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("admin HTML missing %q", need)
		}
	}
}

func TestHomePageRendersRolesAndLogin(t *testing.T) {
	html := webtpl.RenderHome()
	for _, need := range []string{
		"HarborClass",
		"Student",
		"Teacher",
		"Dispatcher",
		"Admin",
		"Sign in",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("home HTML missing %q", need)
		}
	}
}
