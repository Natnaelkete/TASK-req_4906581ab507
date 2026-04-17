// Package handlers hosts the Gin HTTP handlers for every HarborClass
// route. It is intentionally compact — each handler is a thin layer
// over a service from internal/{order,dispatch,notify,audit,auth,store}.
package handlers

import (
	"github.com/eaglepoint/harborclass/internal/audit"
	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/config"
	"github.com/eaglepoint/harborclass/internal/dispatch"
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
