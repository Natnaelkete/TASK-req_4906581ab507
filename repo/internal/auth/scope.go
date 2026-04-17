package auth

import (
	"github.com/eaglepoint/harborclass/internal/models"
)

// Action identifies a business operation checked by the authoriser.
type Action string

const (
	ActionViewSessions      Action = "view_sessions"
	ActionCreateBooking     Action = "create_booking"
	ActionManageOwnOrder    Action = "manage_own_order"
	ActionApproveCancel     Action = "approve_cancel"
	ActionApproveRefund     Action = "approve_refund"
	ActionAssignCourier     Action = "assign_courier"
	ActionManageContent     Action = "manage_content"
	ActionExportAudit       Action = "export_audit"
	ActionSearchAudit       Action = "search_audit"
	ActionConfigureCheckin  Action = "configure_checkin"
	ActionManageMembership  Action = "manage_membership"
	ActionSendNotifications Action = "send_notifications"
)

// Subject is the authenticated user carrying auth claims.
type Subject struct {
	User models.User
}

// Target describes what the Action is being performed against.
type Target struct {
	OrgID    string
	ClassID  string
	OwnerID  string
}

// Can returns true if the subject is allowed to perform the action on
// the target. Both role and org/class scope are enforced.
func Can(sub Subject, act Action, tgt Target) bool {
	u := sub.User
	if u.Role == models.RoleAdmin {
		// Admins are org-scoped: they can act within their own org only.
		return sameOrg(u, tgt)
	}
	switch act {
	case ActionViewSessions:
		return u.Role == models.RoleStudent || u.Role == models.RoleTeacher
	case ActionCreateBooking:
		return u.Role == models.RoleStudent && sameOrg(u, tgt)
	case ActionManageOwnOrder:
		return (u.Role == models.RoleStudent && u.ID == tgt.OwnerID) ||
			(u.Role == models.RoleTeacher && sameOrg(u, tgt))
	case ActionApproveCancel:
		return u.Role == models.RoleTeacher && sameOrg(u, tgt) && memberOfClass(u, tgt.ClassID)
	case ActionApproveRefund:
		// By default only admins approve refunds; teachers can be granted
		// per-org via admin configuration (tracked separately).
		return false
	case ActionAssignCourier:
		return u.Role == models.RoleDispatcher && sameOrg(u, tgt)
	case ActionManageContent:
		return u.Role == models.RoleTeacher && u.ID == tgt.OwnerID
	case ActionExportAudit, ActionSearchAudit:
		// Admin-only above; non-admins denied.
		return false
	case ActionConfigureCheckin, ActionManageMembership:
		return false
	case ActionSendNotifications:
		return u.Role == models.RoleTeacher || u.Role == models.RoleDispatcher
	}
	return false
}

func sameOrg(u models.User, t Target) bool {
	if t.OrgID == "" {
		return true
	}
	return u.OrgID == t.OrgID
}

func memberOfClass(u models.User, classID string) bool {
	if classID == "" {
		return true
	}
	for _, c := range u.ClassIDs {
		if c == classID {
			return true
		}
	}
	return false
}
