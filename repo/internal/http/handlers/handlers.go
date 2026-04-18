// Package handlers hosts the Gin HTTP handlers for every HarborClass
// route. It is intentionally compact — each handler is a thin layer
// over a service from internal/{order,dispatch,notify,audit,auth,store}.
package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/audit"
	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/config"
	"github.com/eaglepoint/harborclass/internal/dispatch"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/notify"
	"github.com/eaglepoint/harborclass/internal/order"
	"github.com/eaglepoint/harborclass/internal/store"
)

// Deps is the constructor input for Handlers.
type Deps struct {
	Config   config.Config
	Store    store.Store
	Auth     *auth.Service
	Machine  *order.Machine
	Engine   *notify.Engine
	Chain    *audit.Chain
	Strategy dispatch.StrategyName
}

// Handlers groups every HTTP handler; it is routed from router.go.
type Handlers struct {
	cfg      config.Config
	store    store.Store
	auth     *auth.Service
	machine  *order.Machine
	engine   *notify.Engine
	chain    *audit.Chain
	strategy dispatch.StrategyName
}

// New creates the handler group.
func New(d Deps) *Handlers {
	return &Handlers{
		cfg: d.Config, store: d.Store, auth: d.Auth, machine: d.Machine,
		engine: d.Engine, chain: d.Chain, strategy: d.Strategy,
	}
}

// can wraps auth.Can with the admin-configured permission overlay that
// lives in the store, so any runtime change to permissions is honoured
// without a restart.
func (h *Handlers) can(c *gin.Context, u models.User, action auth.Action, target auth.Target) bool {
	sub := auth.Subject{User: u}
	if u.OrgID != "" {
		perms, err := h.store.ListPermissions(c.Request.Context(), u.OrgID)
		if err == nil && len(perms) > 0 {
			sub.Overlay = auth.BuildOverlay(perms)
		}
	}
	return auth.Can(sub, action, target)
}
