package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/order"
)

// ListSessions serves the catalog of available sessions, scoped to
// the authenticated caller's organisation. Sessions without an OrgID
// are treated as shared/public catalog entries so legacy fixtures
// remain visible.
func (h *Handlers) ListSessions(c *gin.Context) {
	u := currentUser(c)
	all, err := h.store.ListSessions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]models.Session, 0, len(all))
	for _, s := range all {
		if s.OrgID == "" || s.OrgID == u.OrgID {
			out = append(out, s)
		}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

// CreateBooking creates a new booking in the "created" state and then
// confirms it inline once capacity is reserved.
func (h *Handlers) CreateBooking(c *gin.Context) {
	u := currentUser(c)
	if !h.can(c, u, auth.ActionCreateBooking, auth.Target{OrgID: u.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	s, err := h.store.SessionByID(c.Request.Context(), req.SessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	// Enforce tenant boundary: a student can only book sessions that
	// belong to their own organisation (and class, if the session is
	// class-scoped). Sessions without an OrgID are treated as legacy /
	// cross-org catalog entries to preserve backwards compatibility.
	if s.OrgID != "" && s.OrgID != u.OrgID {
		c.JSON(http.StatusForbidden, gin.H{"error": "session not in your organisation"})
		return
	}
	if s.ClassID != "" && !userInClass(u, s.ClassID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this class"})
		return
	}
	if err := h.store.IncrementSessionBookings(c.Request.Context(), s.ID, 1); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "session full"})
		return
	}
	seq, _ := h.store.CountDailyOrders(c.Request.Context(), time.Now())
	o := h.machine.Create(models.Order{
		ID:        fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		Number:    order.GenerateNumber(time.Now(), seq+1),
		Kind:      models.OrderBooking,
		Payment:   models.PayUnpaid,
		StudentID: u.ID,
		TeacherID: s.TeacherID,
		SessionID: s.ID,
		OrgID:     u.OrgID,
		ClassID:   s.ClassID,
		PickupAt:  s.StartsAt,
		CreatedAt: time.Now(),
	})
	// auto-confirm pending bookings so the student timeline reflects reality.
	if confirmed, err := h.machine.Confirm(o, u.Username); err == nil {
		o = confirmed
	}
	if err := h.store.CreateOrder(c.Request.Context(), o); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), o.OrgID, u.Username, "booking.create", o.ID, o.Number)
	c.JSON(http.StatusCreated, bookingResponse(o))
}

// GetBooking returns a single booking with its timeline. Access is
// routed through the authoriser so every caller — student, teacher,
// admin, dispatcher — is checked against the order's org/class/owner.
// Students must own the order, teachers/admins must be in the same org,
// and any other role is denied unless an overlay grants it.
func (h *Handlers) GetBooking(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionManageOwnOrder, auth.Target{OwnerID: o.StudentID, OrgID: o.OrgID, ClassID: o.ClassID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, bookingResponse(o))
}

// RescheduleBooking moves a confirmed booking up to two times.
func (h *Handlers) RescheduleBooking(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionManageOwnOrder, auth.Target{OwnerID: o.StudentID, OrgID: o.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		NewStart time.Time `json:"new_start"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	updated, err := h.machine.Reschedule(o, u.Username, req.NewStart)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "booking.reschedule", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// CancelBooking cancels a booking, enforcing the 24h teacher-approval rule.
func (h *Handlers) CancelBooking(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionManageOwnOrder, auth.Target{OwnerID: o.StudentID, OrgID: o.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	approver := u.Role == models.RoleTeacher || u.Role == models.RoleAdmin
	sess, sErr := h.store.SessionByID(c.Request.Context(), o.SessionID)
	sessionStart := o.PickupAt
	if sErr == nil {
		sessionStart = sess.StartsAt
	}
	updated, err := h.machine.Cancel(o, u.Username, approver, sessionStart)
	if err != nil {
		if errors.Is(err, order.ErrNeedsApproval) {
			updated.State = models.StatePending
			_ = h.store.UpdateOrder(c.Request.Context(), updated)
			c.JSON(http.StatusAccepted, gin.H{"state": updated.State, "message": "awaits teacher approval"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.store.IncrementSessionBookings(c.Request.Context(), o.SessionID, -1)
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "booking.cancel", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// CompleteBooking transitions a confirmed/in-progress booking to
// completed. The teacher who owns the session or an admin in the same
// org may mark completion; this is the gateway to the 7-day refund
// window enforced by RefundRequest.
func (h *Handlers) CompleteBooking(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if o.Kind != models.OrderBooking {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a booking"})
		return
	}
	allowed := (u.Role == models.RoleTeacher && u.OrgID == o.OrgID && u.ID == o.TeacherID) ||
		(u.Role == models.RoleAdmin && u.OrgID == o.OrgID)
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	updated, err := h.machine.Complete(o, u.Username)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "booking.complete", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// RefundRequest files a refund within the 7-day post-completion window.
func (h *Handlers) RefundRequest(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionManageOwnOrder, auth.Target{OwnerID: o.StudentID, OrgID: o.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	updated, err := h.machine.RequestRefund(o, u.Username)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateOrder(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "booking.refund-request", updated.ID, updated.Number)
	c.JSON(http.StatusOK, bookingResponse(updated))
}

// MyOrders lists the current student's orders.
func (h *Handlers) MyOrders(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	list, err := h.store.ListOrdersByStudent(c.Request.Context(), u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, o := range list {
		out = append(out, bookingResponse(o))
	}
	c.JSON(http.StatusOK, gin.H{"orders": out})
}

// UpdateSubscription toggles the subscription flag for a category.
func (h *Handlers) UpdateSubscription(c *gin.Context) {
	u := currentUser(c)
	var req struct {
		Category   string `json:"category"`
		Subscribed bool   `json:"subscribed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.store.SetSubscription(c.Request.Context(), models.Subscription{UserID: u.ID, Category: req.Category, Subscribed: req.Subscribed}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "subscription.update", "notify:"+req.Category, boolStr(req.Subscribed))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// OneClickUnsubscribe supports opt-out via a plain URL
// (?user=&category=&token=). The token is an HMAC-signed expiring
// value generated by the engine when the reminder was sent; this
// prevents unauthenticated third parties from toggling arbitrary
// users' subscriptions.
func (h *Handlers) OneClickUnsubscribe(c *gin.Context) {
	userID := c.Query("user")
	cat := c.Query("category")
	token := c.Query("token")
	if userID == "" || cat == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing params"})
		return
	}
	key := auth.DeriveKey(h.cfg.EncryptionKey)
	if err := auth.VerifyUnsubscribe(key, userID, cat, token, time.Now()); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.SetSubscription(c.Request.Context(), models.Subscription{UserID: userID, Category: cat, Subscribed: false}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	orgID := ""
	if target, err := h.store.UserByID(c.Request.Context(), userID); err == nil {
		orgID = target.OrgID
	}
	_, _ = h.chain.Append(c.Request.Context(), orgID, userID, "subscription.unsubscribe", "notify:"+cat, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func bookingResponse(o models.Order) gin.H {
	return gin.H{
		"id":               o.ID,
		"number":           o.Number,
		"kind":             o.Kind,
		"state":            o.State,
		"payment":          o.Payment,
		"student_id":       o.StudentID,
		"session_id":       o.SessionID,
		"courier_id":       o.CourierID,
		"reschedule_count": o.RescheduleCount,
		"timeline":         o.Timeline,
	}
}

func userInClass(u models.User, classID string) bool {
	for _, c := range u.ClassIDs {
		if c == classID {
			return true
		}
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

