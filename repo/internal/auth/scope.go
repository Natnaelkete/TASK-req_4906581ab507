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
	ActionScheduleException Action = "schedule_exception"
)

// Subject is the authenticated user carrying auth claims. The Overlay
// map, if non-nil, carries admin-configured per-action role grants for
// the subject's organisation. When a row for an action exists here it
// REPLACES the static default (strict overlay semantics).
type Subject struct {
	User    models.User
	Overlay map[Action][]models.Role
}

// Target describes what the Action is being performed against.
type Target struct {
	OrgID    string
	ClassID  string
	OwnerID  string
}

// Can returns true if the subject is allowed to perform the action on
// the target. Both role and org/class scope are enforced. If an admin
// has installed an overlay entry for the action, that entry is applied
// in addition to the scope constraints (never expands to foreign orgs).
func Can(sub Subject, act Action, tgt Target) bool {
	u := sub.User

	// Dynamic permission overlay: if an admin installed an entry for
	// this action inside the subject's org, those roles are authoritative
	// for the role gate. Scope (same-org / class / owner) still applies.
	if allowed, ok := sub.Overlay[act]; ok {
		if !roleAllowed(u.Role, allowed) {
			return false
		}
		return scopeAllowed(u, act, tgt)
	}

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
		// per-org via admin configuration.
		return false
	case ActionAssignCourier:
		return u.Role == models.RoleDispatcher && sameOrg(u, tgt)
	case ActionManageContent:
		return u.Role == models.RoleTeacher && u.ID == tgt.OwnerID
	case ActionExportAudit, ActionSearchAudit:
		// Admin-only above; non-admins denied.
		return false
	case ActionConfigureCheckin, ActionManageMembership, ActionScheduleException:
		return false
	case ActionSendNotifications:
		return (u.Role == models.RoleTeacher || u.Role == models.RoleDispatcher) && sameOrg(u, tgt)
	}
	return false
}

func scopeAllowed(u models.User, act Action, tgt Target) bool {
	switch act {
	case ActionManageOwnOrder:
		if u.Role == models.RoleStudent {
			return u.ID == tgt.OwnerID
		}
		return sameOrg(u, tgt)
	case ActionApproveCancel:
		return sameOrg(u, tgt) && memberOfClass(u, tgt.ClassID)
	case ActionManageContent:
		return u.ID == tgt.OwnerID
	default:
		return sameOrg(u, tgt)
	}
}

func roleAllowed(r models.Role, allowed []models.Role) bool {
	for _, a := range allowed {
		if a == r {
			return true
		}
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

// BuildOverlay converts a list of admin-configured permissions into the
// map shape used by Subject. Unknown actions are ignored; duplicates
// are merged.
func BuildOverlay(permissions []models.Permission) map[Action][]models.Role {
	if len(permissions) == 0 {
		return nil
	}
	out := map[Action][]models.Role{}
	for _, p := range permissions {
		act := Action(p.Action)
		for _, r := range p.Roles {
			out[act] = append(out[act], models.Role(r))
		}
	}
	return out
}
