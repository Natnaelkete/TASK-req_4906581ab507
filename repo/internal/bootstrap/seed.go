// Package bootstrap seeds deterministic demo data: one organisation,
// one class, and one demo account per role. These are the credentials
// documented in README.md so a static auditor can verify both.
package bootstrap

import (
	"context"
	"time"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/config"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/notify"
	"github.com/eaglepoint/harborclass/internal/store"
)

// DemoUsers lists every seeded account. README documents the same list.
type DemoUser struct {
	Username    string
	Password    string
	DisplayName string
	Role        models.Role
	Rating      float64
	Load        int
}

// Users returns the canonical demo account set. Keep synchronised with
// README's Demo credentials table.
func Users() []DemoUser {
	return []DemoUser{
		{"student", "student-pass", "Alex Student", models.RoleStudent, 0, 0},
		{"teacher", "teacher-pass", "Taylor Teacher", models.RoleTeacher, 0, 0},
		{"courier", "courier-pass", "Casey Courier", models.RoleCourier, 4.9, 2},
		{"dispatcher", "dispatch-pass", "Dana Dispatcher", models.RoleDispatcher, 0, 0},
		{"admin", "admin-pass", "Avery Admin", models.RoleAdmin, 0, 0},
	}
}

// Seed runs every seeder once. It is safe to call repeatedly.
func Seed(ctx context.Context, s store.Store, cfg config.Config) error {
	key := auth.DeriveKey(cfg.EncryptionKey)
	for _, du := range Users() {
		if _, err := s.UserByUsername(ctx, du.Username); err == nil {
			continue
		}
		phone, _ := auth.EncryptPII(key, "+1-555-010-"+du.Username)
		u := models.User{
			ID:           "usr-" + du.Username,
			Username:     du.Username,
			Role:         du.Role,
			OrgID:        "org-main",
			ClassIDs:     []string{"class-default"},
			PasswordHash: auth.HashPassword(du.Password, "harborclass-demo"),
			PhoneCipher:  phone,
			DisplayName:  du.DisplayName,
			Rating:       du.Rating,
			Load:         du.Load,
			Location:     models.Location{Lat: 40.7, Lng: -74.0},
			CreatedAt:    time.Now(),
		}
		_ = s.CreateUser(ctx, u)
	}
	_ = s.UpsertFacility(ctx, models.Facility{
		ID:               "fac-main",
		Name:             "Main site",
		BlacklistedZones: []string{"restricted-north"},
		PickupCutoffHour: cfg.PickupCutoffHour,
	})
	_ = s.CreateSession(ctx, models.Session{
		ID:        "sess-demo-1",
		TeacherID: "usr-teacher",
		ClassID:   "class-default",
		OrgID:     "org-main",
		Title:     "Intro to HarborClass",
		StartsAt:  time.Now().Add(48 * time.Hour),
		EndsAt:    time.Now().Add(49 * time.Hour),
		Capacity:  20,
	})
	seedTeacherContent(ctx, s, "usr-teacher")
	return notify.SeedTemplates(ctx, s)
}

// seedTeacherContent installs a handful of demo content items so that
// the teacher console shows realistic pinned posts and non-zero
// analytics windows on first boot.
func seedTeacherContent(ctx context.Context, s store.Store, teacherID string) {
	now := time.Now()
	items := []models.ContentItem{
		{
			ID: "content-welcome", TeacherID: teacherID, Title: "Welcome post",
			Body: "Welcome to the studio.",
			Pinned: true, Published: true,
			Views: 420, Likes: 57, Favorites: 12, Followers: 4,
			CreatedAt: now.Add(-3 * 24 * time.Hour), UpdatedAt: now.Add(-3 * 24 * time.Hour),
		},
		{
			ID: "content-starter-kit", TeacherID: teacherID, Title: "Starter kit",
			Body: "Everything new students need to know.",
			Pinned: true, Published: true,
			Views: 1480, Likes: 183, Favorites: 46, Followers: 13,
			CreatedAt: now.Add(-20 * 24 * time.Hour), UpdatedAt: now.Add(-20 * 24 * time.Hour),
		},
		{
			ID: "content-archive", TeacherID: teacherID, Title: "Last quarter retrospective",
			Body: "Recap of last quarter.",
			Pinned: false, Published: true,
			Views: 3940, Likes: 372, Favorites: 121, Followers: 24,
			CreatedAt: now.Add(-80 * 24 * time.Hour), UpdatedAt: now.Add(-80 * 24 * time.Hour),
		},
	}
	for _, it := range items {
		_ = s.UpsertContent(ctx, it)
	}
}
