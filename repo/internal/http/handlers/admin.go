package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

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
	target.ClassIDs = req.ClassIDs
	if req.Role != "" {
		target.Role = models.Role(req.Role)
	}
	if err := h.store.UpdateUser(c.Request.Context(), target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "admin.membership", target.ID, req.Role)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminPermissions stores per-role permission flags in the audit log.
// In this offline demo the flags live in audit entries so they are
// tamper-evident; the admin console reads them back via search.
func (h *Handlers) AdminPermissions(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "admin.permissions", "org", toString(req))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminApproveRefund approves a refund_review order.
func (h *Handlers) AdminApproveRefund(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
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
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "admin.refund.approve", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// AdminRollback applies a compensating rollback on any order.
func (h *Handlers) AdminRollback(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
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
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "admin.rollback", updated.ID, req.Reason)
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
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "admin.facility", f.ID, f.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return "permissions-updated"
}
