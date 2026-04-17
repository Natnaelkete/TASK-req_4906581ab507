package webtpl_test

import (
	"strings"
	"testing"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

func TestTeacherDashboardAnalyticsWindows(t *testing.T) {
	d := webtpl.TeacherData{
		User: models.User{DisplayName: "Taylor Teacher", Role: models.RoleTeacher},
		Analytics: webtpl.Analytics{
			Window7:  webtpl.Window{Views: 420, Likes: 57, Favorites: 12, Followers: 4},
			Window30: webtpl.Window{Views: 1900, Likes: 240, Favorites: 58, Followers: 17},
			Window90: webtpl.Window{Views: 5840, Likes: 612, Favorites: 179, Followers: 41},
		},
	}
	html := webtpl.RenderTeacherDashboard(d)
	for _, need := range []string{
		`data-role="teacher"`,
		"Teacher Console",
		"Taylor Teacher",
		"Pin featured work",
		"Bulk edit",
		"Unpublish",
		"Delete",
		"7 days",
		"30 days",
		"90 days",
		"Views",
		"Likes+Favorites",
		"Followers",
		"420",   // 7d views
		"1900",  // 30d views
		"5840",  // 90d views
		"69",    // 7d engagement = 57+12
		"298",   // 30d engagement = 240+58
		"791",   // 90d engagement = 612+179
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("teacher HTML missing %q", need)
		}
	}
}
