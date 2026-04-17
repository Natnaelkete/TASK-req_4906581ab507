package auth_test

import (
	"testing"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
)

func TestStudentScopeChecks(t *testing.T) {
	s := auth.Subject{User: models.User{ID: "u1", Role: models.RoleStudent, OrgID: "o1", ClassIDs: []string{"c1"}}}
	cases := []struct {
		name string
		act  auth.Action
		tgt  auth.Target
		want bool
	}{
		{"student can view sessions", auth.ActionViewSessions, auth.Target{OrgID: "o1"}, true},
		{"student can create booking in own org", auth.ActionCreateBooking, auth.Target{OrgID: "o1"}, true},
		{"student cannot create booking in other org", auth.ActionCreateBooking, auth.Target{OrgID: "other"}, false},
		{"student manages own order", auth.ActionManageOwnOrder, auth.Target{OwnerID: "u1"}, true},
		{"student cannot manage another student's order", auth.ActionManageOwnOrder, auth.Target{OwnerID: "u2"}, false},
		{"student cannot assign courier", auth.ActionAssignCourier, auth.Target{OrgID: "o1"}, false},
		{"student cannot export audit", auth.ActionExportAudit, auth.Target{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.Can(s, tc.act, tc.tgt); got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestTeacherScopeChecks(t *testing.T) {
	s := auth.Subject{User: models.User{ID: "t1", Role: models.RoleTeacher, OrgID: "o1", ClassIDs: []string{"c1"}}}
	if !auth.Can(s, auth.ActionApproveCancel, auth.Target{OrgID: "o1", ClassID: "c1"}) {
		t.Fatal("teacher should approve cancel in own class")
	}
	if auth.Can(s, auth.ActionApproveCancel, auth.Target{OrgID: "other", ClassID: "c1"}) {
		t.Fatal("teacher must not approve cancel in other org")
	}
	if auth.Can(s, auth.ActionApproveCancel, auth.Target{OrgID: "o1", ClassID: "unknown"}) {
		t.Fatal("teacher must not approve cancel in class they don't own")
	}
	if !auth.Can(s, auth.ActionManageContent, auth.Target{OwnerID: "t1"}) {
		t.Fatal("teacher manages own content")
	}
}

func TestDispatcherScopeChecks(t *testing.T) {
	s := auth.Subject{User: models.User{ID: "d1", Role: models.RoleDispatcher, OrgID: "o1"}}
	if !auth.Can(s, auth.ActionAssignCourier, auth.Target{OrgID: "o1"}) {
		t.Fatal("dispatcher should assign in own org")
	}
	if auth.Can(s, auth.ActionAssignCourier, auth.Target{OrgID: "o2"}) {
		t.Fatal("dispatcher cross-org must be denied")
	}
}

func TestAdminOrgScope(t *testing.T) {
	s := auth.Subject{User: models.User{ID: "a1", Role: models.RoleAdmin, OrgID: "o1"}}
	if !auth.Can(s, auth.ActionExportAudit, auth.Target{OrgID: "o1"}) {
		t.Fatal("admin can export audit in own org")
	}
	if auth.Can(s, auth.ActionExportAudit, auth.Target{OrgID: "o2"}) {
		t.Fatal("admin cross-org must be denied")
	}
}

func TestPasswordHashingRoundTrip(t *testing.T) {
	h := auth.HashPassword("secret", "salt-1")
	if !auth.VerifyPassword("secret", h) {
		t.Fatal("verify failed on correct password")
	}
	if auth.VerifyPassword("wrong", h) {
		t.Fatal("verify accepted wrong password")
	}
}

func TestPhoneEncryptionAndMask(t *testing.T) {
	key := auth.DeriveKey("demo-key")
	ct, err := auth.EncryptPII(key, "+1-555-010-student")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := auth.DecryptPII(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "+1-555-010-student" {
		t.Fatalf("round-trip mismatch: %s", pt)
	}
	masked := auth.MaskPhone("+1-555-010-1234")
	// last 4 of 11 digits preserved, rest '*'
	if masked != "*******1234" {
		t.Fatalf("mask wrong: %s", masked)
	}
}

func TestExtractBearerToken(t *testing.T) {
	if auth.ExtractBearerToken("Bearer abc.123") != "abc.123" {
		t.Fatal("bearer parse failed")
	}
	if auth.ExtractBearerToken("Basic xx") != "" {
		t.Fatal("non-bearer should yield empty token")
	}
}
