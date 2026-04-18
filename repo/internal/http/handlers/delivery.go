package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/dispatch"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/order"
)

// ListDeliveries returns delivery-kind orders scoped to the caller's
// organisation. Admin callers see their own org; dispatchers see their
// org. This prevents cross-tenant data exposure on the queue.
func (h *Handlers) ListDeliveries(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleDispatcher && u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	list, err := h.store.ListDeliveriesByOrg(c.Request.Context(), u.OrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": list})
}

// CreateDelivery seeds a delivery-kind order in the caller's org.
func (h *Handlers) CreateDelivery(c *gin.Context) {
	u := currentUser(c)
	if !h.can(c, u, auth.ActionAssignCourier, auth.Target{OrgID: u.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		PickupAt   time.Time `json:"pickup_at"`
		PickupZone string    `json:"pickup_zone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	seq, _ := h.store.CountDailyOrders(c.Request.Context(), time.Now())
	o := h.machine.Create(models.Order{
		ID:         fmt.Sprintf("dlv-%d", time.Now().UnixNano()),
		Number:     order.GenerateNumber(time.Now(), seq+1),
		Kind:       models.OrderDelivery,
		Payment:    models.PayUnpaid,
		OrgID:      u.OrgID,
		PickupAt:   req.PickupAt,
		PickupZone: req.PickupZone,
		CreatedAt:  time.Now(),
	})
	if err := h.store.CreateOrder(c.Request.Context(), o); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), o.OrgID, u.Username, "delivery.create", o.ID, o.Number)
	c.JSON(http.StatusCreated, gin.H{"id": o.ID, "number": o.Number, "state": o.State})
}

// AssignCourier runs the configured strategy to assign a courier. The
// authorisation check uses the order's org, not the caller's, so a
// dispatcher cannot reach into another org's deliveries by guessing ids.
func (h *Handlers) AssignCourier(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !h.can(c, u, auth.ActionAssignCourier, auth.Target{OrgID: o.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Strategy   string `json:"strategy"`
		FacilityID string `json:"facility_id"`
	}
	_ = c.ShouldBindJSON(&req)
	strategy := dispatch.StrategyName(req.Strategy)
	if strategy == "" {
		strategy = h.strategy
	}
	var facility models.Facility
	if req.FacilityID != "" {
		facility, _ = h.store.FacilityByID(c.Request.Context(), req.FacilityID)
	}
	couriers, err := h.store.ListUsersByRole(c.Request.Context(), models.RoleCourier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Restrict candidate couriers to the order's organisation only.
	scoped := make([]models.User, 0, len(couriers))
	for _, co := range couriers {
		if co.OrgID == "" || co.OrgID == o.OrgID {
			scoped = append(scoped, co)
		}
	}
	existing, _ := h.store.ListDeliveriesByOrg(c.Request.Context(), o.OrgID)
	picked, assignErr := dispatch.Assign(strategy, o, facility, scoped, existing)
	if assignErr != nil {
		status := http.StatusConflict
		switch {
		case errors.Is(assignErr, dispatch.ErrNoEligibleCourier):
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": assignErr.Error(), "conflict": true})
		return
	}
	o.CourierID = picked.ID
	o.State = models.StateInProgress
	if err := h.store.UpdateOrder(c.Request.Context(), o); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), o.OrgID, u.Username, "delivery.assign", o.ID, picked.ID)
	c.JSON(http.StatusOK, gin.H{
		"order_id":   o.ID,
		"courier_id": picked.ID,
		"strategy":   strategy,
		"state":      o.State,
	})
}

// CompleteDelivery transitions a delivery order to completed. The
// assigned courier, a same-org dispatcher, or a same-org admin may
// signal completion; the transition opens the 7-day refund window.
func (h *Handlers) CompleteDelivery(c *gin.Context) {
	u := currentUser(c)
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if o.Kind != models.OrderDelivery {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a delivery"})
		return
	}
	allowed := (u.Role == models.RoleCourier && u.ID == o.CourierID) ||
		((u.Role == models.RoleDispatcher || u.Role == models.RoleAdmin) && u.OrgID == o.OrgID)
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
	_, _ = h.chain.Append(c.Request.Context(), updated.OrgID, u.Username, "delivery.complete", updated.ID, updated.Number)
	c.JSON(http.StatusOK, gin.H{"order_id": updated.ID, "state": updated.State})
}
