package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/notify"
)

// SendNotification dispatches a template to a recipient. Recipient must
// be in the caller's organisation; otherwise the call is rejected as
// cross-tenant and never touches the message transport.
func (h *Handlers) SendNotification(c *gin.Context) {
	u := currentUser(c)
	var req struct {
		OrderID    string `json:"order_id"`
		UserID     string `json:"user_id"`
		Category   string `json:"category"`
		TemplateID string `json:"template_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	recipient, err := h.store.UserByID(c.Request.Context(), req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recipient not found"})
		return
	}
	// Authorise sending against the recipient's org (not the sender's).
	// This blocks cross-tenant recipient enumeration.
	if !h.can(c, u, auth.ActionSendNotifications, auth.Target{OrgID: recipient.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	res, sendErr := h.engine.Send(c.Request.Context(), notify.SendRequest{
		OrderID:    req.OrderID,
		UserID:     req.UserID,
		Recipient:  recipient,
		Category:   req.Category,
		TemplateID: req.TemplateID,
	})
	// Always surface a signed one-click unsubscribe URL so callers (or
	// the outbound message template) can embed it.
	key := auth.DeriveKey(h.cfg.EncryptionKey)
	token := auth.SignUnsubscribe(key, req.UserID, req.Category, time.Now())
	if sendErr != nil {
		status := http.StatusConflict
		switch {
		case errors.Is(sendErr, notify.ErrRateLimited):
			status = http.StatusTooManyRequests
		case errors.Is(sendErr, notify.ErrUnsubscribed):
			status = http.StatusForbidden
		case errors.Is(sendErr, notify.ErrTemplateMissing):
			status = http.StatusNotFound
		case errors.Is(sendErr, notify.ErrMaxRetries):
			status = http.StatusBadGateway
		}
		_, _ = h.chain.Append(c.Request.Context(), recipient.OrgID, u.Username, "notify.send.failed", req.OrderID, sendErr.Error())
		c.JSON(status, gin.H{
			"error":    sendErr.Error(),
			"attempts": res.Attempts,
			"success":  res.Success,
		})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), recipient.OrgID, u.Username, "notify.send", req.OrderID, req.TemplateID)
	c.JSON(http.StatusOK, gin.H{
		"attempts":         res.Attempts,
		"success":          res.Success,
		"unsubscribe_url":  "/api/my/subscriptions/unsubscribe?user=" + req.UserID + "&category=" + req.Category + "&token=" + token,
	})
}

// ListTemplates lists stored notification templates.
func (h *Handlers) ListTemplates(c *gin.Context) {
	list, err := h.store.ListTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"templates": list})
}

// UpsertTemplate stores or replaces a notification template.
func (h *Handlers) UpsertTemplate(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var t models.NotificationTemplate
	if err := c.ShouldBindJSON(&t); err != nil || t.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.store.UpsertTemplate(c.Request.Context(), t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "notify.template.upsert", t.ID, t.Category)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
