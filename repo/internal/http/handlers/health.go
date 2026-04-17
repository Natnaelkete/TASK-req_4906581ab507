package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/http/middleware"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// Health is the API health check used by docker-compose healthchecks
// and the admin observability dashboard.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "harborclass",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// Metrics returns a compact JSON view of request counters.
func (h *Handlers) Metrics(c *gin.Context) {
	req, errs := middleware.Metrics()
	c.JSON(http.StatusOK, gin.H{
		"requests": req,
		"errors":   errs,
	})
}

// Alerts returns the last admin alerts (offline, in-process).
// Alerts are synthesised from audit entries whose action starts with
// "notify.send.failed"; that gives admins a ready-made offline feed
// without external infrastructure.
func (h *Handlers) Alerts(c *gin.Context) {
	rows, _ := h.chain.Search(c.Request.Context(), store.AuditFilter{})
	out := []gin.H{}
	for _, r := range rows {
		if r.Action == "notify.send.failed" {
			out = append(out, gin.H{"at": r.At, "detail": r.Detail, "resource": r.Resource})
		}
	}
	c.JSON(http.StatusOK, gin.H{"alerts": out, "count": len(out)})
}

// CrashReport persists a client-reported crash into the audit log.
func (h *Handlers) CrashReport(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
		Stack   string `json:"stack"`
		Actor   string `json:"actor"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), req.Actor, "crash", "client:"+req.Version, req.Stack)
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}

// RegisterDevice registers or updates a managed device.
func (h *Handlers) RegisterDevice(c *gin.Context) {
	u := currentUser(c)
	var d models.Device
	if err := c.ShouldBindJSON(&d); err != nil || d.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	d.UserID = u.ID
	d.LastSeen = time.Now()
	if err := h.store.UpsertDevice(c.Request.Context(), d); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DevicePolicy returns the upgrade decision for a given device.
// Canary devices get the latest build; out-of-date devices receive a
// forced-upgrade instruction.
func (h *Handlers) DevicePolicy(c *gin.Context) {
	version := c.Query("version")
	devices, _ := h.store.ListDevices(c.Request.Context())
	decision := gin.H{"upgrade_required": false, "forced_version": "", "canary": false}
	for _, d := range devices {
		if d.ID == c.Query("device_id") {
			if d.ForcedUpgradeTo != "" && d.ForcedUpgradeTo != version {
				decision["upgrade_required"] = true
				decision["forced_version"] = d.ForcedUpgradeTo
			}
			decision["canary"] = d.Canary
			break
		}
	}
	c.JSON(http.StatusOK, decision)
}
