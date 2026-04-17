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
		Title:     "Intro to HarborClass",
		StartsAt:  time.Now().Add(48 * time.Hour),
		EndsAt:    time.Now().Add(49 * time.Hour),
		Capacity:  20,
	})
	return notify.SeedTemplates(ctx, s)
}
