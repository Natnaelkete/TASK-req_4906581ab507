package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
)

// AdminMembership adjusts org/class membership rows.
func (h *Handlers) AdminMembership(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Username string   `json:"username"`
		ClassIDs []string `json:"class_ids"`
		Role     string   `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	target, err := h.store.UserByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	// Admins are org-scoped: cannot reach into other orgs.
	if target.OrgID != u.OrgID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cross-org not allowed"})
		return
	}
	target.ClassIDs = req.ClassIDs
	if req.Role != "" {
		if !isValidRole(req.Role) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		target.Role = models.Role(req.Role)
	}
	if err := h.store.UpdateUser(c.Request.Context(), target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "admin.membership", target.ID, req.Role)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// isValidRole rejects role strings outside the canonical enum so
// authorisation behaviour cannot drift via arbitrary writes.
func isValidRole(r string) bool {
	switch models.Role(r) {
	case models.RoleStudent, models.RoleTeacher, models.RoleCourier,
		models.RoleDispatcher, models.RoleAdmin:
		return true
	}
	return false
}

// AdminPermissions persists per-action role grants that overlay the
// static matrix in auth.Can. Subsequent authorisation checks for any
// listed action are gated by this stored permission.
func (h *Handlers) AdminPermissions(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Permissions []struct {
			Action string   `json:"action"`
			Roles  []string `json:"roles"`
		} `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if len(req.Permissions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty permissions"})
		return
	}
	for _, p := range req.Permissions {
		if p.Action == "" {
			continue
		}
		perm := models.Permission{OrgID: u.OrgID, Action: p.Action, Roles: p.Roles}
		if err := h.store.UpsertPermission(c.Request.Context(), perm); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "admin.permissions",
			"org:"+u.OrgID+":"+p.Action, strings.Join(p.Roles, ","))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(req.Permissions)})
}

// AdminApproveRefund approves a refund_review order. Authorisation
// flows through h.can so admins can extend ActionApproveRefund to other
// roles (e.g. teachers in the same org) via the permission overlay.
// Cross-org reach remains impossible — scopeAllowed enforces sameOrg.
func (h *Handlers) AdminApproveRefund(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionApproveRefund, auth.Target{OrgID: o.OrgID, ClassID: o.ClassID, OwnerID: o.StudentID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	updated, err := h.machine.ApproveRefund(o, u.Username)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "admin.refund.approve", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// AdminRollback applies a compensating rollback on any order.
func (h *Handlers) AdminRollback(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if u.Role != models.RoleAdmin || (o.OrgID != "" && o.OrgID != u.OrgID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct{ Reason string }
	_ = c.ShouldBindJSON(&req)
	updated, err := h.machine.Rollback(o, u.Username, req.Reason)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "admin.rollback", updated.ID, req.Reason)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// AdminFacility upserts a facility (zone blacklist / cutoff hour).
func (h *Handlers) AdminFacility(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var f models.Facility
	if err := c.ShouldBindJSON(&f); err != nil || f.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.store.UpsertFacility(c.Request.Context(), f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "admin.facility", f.ID, f.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
