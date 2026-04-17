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

// ListDeliveries returns all delivery-kind orders.
func (h *Handlers) ListDeliveries(c *gin.Context) {
	list, err := h.store.ListDeliveries(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": list})
}

// CreateDelivery seeds a delivery-kind order (used by tests & admin UI).
func (h *Handlers) CreateDelivery(c *gin.Context) {
	u := currentUser(c)
	if !auth.Can(auth.Subject{User: u}, auth.ActionAssignCourier, auth.Target{OrgID: u.OrgID}) {
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
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "delivery.create", o.ID, o.Number)
	c.JSON(http.StatusCreated, gin.H{"id": o.ID, "number": o.Number, "state": o.State})
}

// AssignCourier runs the configured strategy to assign a courier.
func (h *Handlers) AssignCourier(c *gin.Context) {
	u := currentUser(c)
	if !auth.Can(auth.Subject{User: u}, auth.ActionAssignCourier, auth.Target{OrgID: u.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	o, err := h.store.OrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
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
	existing, _ := h.store.ListDeliveries(c.Request.Context())
	picked, assignErr := dispatch.Assign(strategy, o, facility, couriers, existing)
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
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "delivery.assign", o.ID, picked.ID)
	c.JSON(http.StatusOK, gin.H{
		"order_id":   o.ID,
		"courier_id": picked.ID,
		"strategy":   strategy,
		"state":      o.State,
	})
}
