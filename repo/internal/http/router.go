// Package http wires Gin routing, middleware, and handlers into a
// single Engine that both cmd/harborclass and tests construct.
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/audit"
	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/config"
	"github.com/eaglepoint/harborclass/internal/dispatch"
	"github.com/eaglepoint/harborclass/internal/http/handlers"
	"github.com/eaglepoint/harborclass/internal/http/middleware"
	"github.com/eaglepoint/harborclass/internal/notify"
	"github.com/eaglepoint/harborclass/internal/order"
	"github.com/eaglepoint/harborclass/internal/store"
)

// Dependencies holds the wired services a router needs.
type Dependencies struct {
	Config   config.Config
	Store    store.Store
	Auth     *auth.Service
	Machine  *order.Machine
	Engine   *notify.Engine
	Chain    *audit.Chain
	Strategy dispatch.StrategyName
}

// NewRouter returns a Gin engine with every HarborClass route wired.
// No mocking is used; the same handlers, services, and repositories
// run in production and tests.
func NewRouter(d Dependencies) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestLogger(), middleware.Observability())

	h := handlers.New(handlers.Deps{
		Store:    d.Store,
		Auth:     d.Auth,
		Machine:  d.Machine,
		Engine:   d.Engine,
		Chain:    d.Chain,
		Strategy: d.Strategy,
		Config:   d.Config,
	})

	// Health & ops
	r.GET("/api/health", h.Health)
	r.GET("/api/metrics", h.Metrics)
	r.GET("/api/alerts", h.Alerts)
	r.POST("/api/crash-reports", h.CrashReport)

	// Auth
	r.POST("/api/auth/login", h.Login)
	r.POST("/api/auth/logout", middleware.RequireAuth(d.Auth), h.Logout)
	r.GET("/api/auth/whoami", middleware.RequireAuth(d.Auth), h.WhoAmI)

	// Student & teacher flows
	r.GET("/api/sessions", h.ListSessions)
	r.POST("/api/bookings", middleware.RequireAuth(d.Auth), h.CreateBooking)
	r.GET("/api/bookings/:id", middleware.RequireAuth(d.Auth), h.GetBooking)
	r.POST("/api/bookings/:id/reschedule", middleware.RequireAuth(d.Auth), h.RescheduleBooking)
	r.POST("/api/bookings/:id/cancel", middleware.RequireAuth(d.Auth), h.CancelBooking)
	r.POST("/api/bookings/:id/refund-request", middleware.RequireAuth(d.Auth), h.RefundRequest)
	r.GET("/api/my/orders", middleware.RequireAuth(d.Auth), h.MyOrders)
	r.POST("/api/my/subscriptions", middleware.RequireAuth(d.Auth), h.UpdateSubscription)
	r.GET("/api/my/subscriptions/unsubscribe", h.OneClickUnsubscribe)

	// Teacher content console
	r.GET("/api/teacher/profile", middleware.RequireAuth(d.Auth), h.TeacherProfile)
	r.POST("/api/teacher/pin", middleware.RequireAuth(d.Auth), h.TeacherPin)
	r.POST("/api/teacher/content/bulk", middleware.RequireAuth(d.Auth), h.TeacherBulk)
	r.GET("/api/teacher/analytics", middleware.RequireAuth(d.Auth), h.TeacherAnalytics)

	// Dispatch
	r.GET("/api/deliveries", middleware.RequireAuth(d.Auth), h.ListDeliveries)
	r.POST("/api/deliveries", middleware.RequireAuth(d.Auth), h.CreateDelivery)
	r.POST("/api/deliveries/:id/assign", middleware.RequireAuth(d.Auth), h.AssignCourier)

	// Notifications
	r.POST("/api/notifications/send", middleware.RequireAuth(d.Auth), h.SendNotification)
	r.GET("/api/notifications/templates", middleware.RequireAuth(d.Auth), h.ListTemplates)
	r.POST("/api/notifications/templates", middleware.RequireAuth(d.Auth), h.UpsertTemplate)

	// Admin
	r.POST("/api/admin/membership", middleware.RequireAuth(d.Auth), h.AdminMembership)
	r.POST("/api/admin/permissions", middleware.RequireAuth(d.Auth), h.AdminPermissions)
	r.POST("/api/admin/refunds/:id/approve", middleware.RequireAuth(d.Auth), h.AdminApproveRefund)
	r.POST("/api/admin/orders/:id/rollback", middleware.RequireAuth(d.Auth), h.AdminRollback)
	r.POST("/api/admin/facilities", middleware.RequireAuth(d.Auth), h.AdminFacility)

	// Audit
	r.GET("/api/audit-logs", middleware.RequireAuth(d.Auth), h.SearchAudit)
	r.GET("/api/audit-logs/export", middleware.RequireAuth(d.Auth), h.ExportAudit)

	// Devices (canary / forced upgrade)
	r.POST("/api/devices/register", middleware.RequireAuth(d.Auth), h.RegisterDevice)
	r.GET("/api/devices/policy", middleware.RequireAuth(d.Auth), h.DevicePolicy)

	// Server-rendered UI (Go Templ)
	r.GET("/", h.Home)
	r.GET("/student", middleware.RequireAuth(d.Auth), h.StudentPage)
	r.GET("/teacher", middleware.RequireAuth(d.Auth), h.TeacherPage)
	r.GET("/dispatcher", middleware.RequireAuth(d.Auth), h.DispatcherPage)
	r.GET("/admin", middleware.RequireAuth(d.Auth), h.AdminPage)

	return r
}
