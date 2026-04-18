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

// Metrics returns a compact JSON view of request counters. The
// operational counters are admin-only: routine users do not need them
// and exposing them broadens the attack surface of the observability
// console.
func (h *Handlers) Metrics(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	req, errs := middleware.Metrics()
	c.JSON(http.StatusOK, gin.H{
		"requests": req,
		"errors":   errs,
	})
}

// Alerts returns the last admin alerts (offline, in-process).
// Alerts are synthesised from audit entries whose action is
// "notify.send.failed"; that gives admins a ready-made offline feed
// without external infrastructure. Only admins see alerts, and the
// feed is scoped to the admin's organisation so cross-tenant failures
// stay isolated.
func (h *Handlers) Alerts(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	rows, _ := h.chain.Search(c.Request.Context(), store.AuditFilter{OrgID: u.OrgID})
	out := []gin.H{}
	for _, r := range rows {
		if r.Action == "notify.send.failed" {
			out = append(out, gin.H{"at": r.At, "detail": r.Detail, "resource": r.Resource})
		}
	}
	c.JSON(http.StatusOK, gin.H{"alerts": out, "count": len(out)})
}

// maxCrashPayloadBytes caps the amount of free-form crash data persisted
// into the audit log so an authenticated caller cannot flood the chain.
const maxCrashPayloadBytes = 16 * 1024

// CrashReport persists a client-reported crash into the audit log.
// Actor is taken from the authenticated session (the caller's own
// username) rather than the request body to prevent log spoofing.
func (h *Handlers) CrashReport(c *gin.Context) {
	u := currentUser(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCrashPayloadBytes)
	var req struct {
		Version string `json:"version"`
		Stack   string `json:"stack"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if len(req.Stack) > maxCrashPayloadBytes {
		req.Stack = req.Stack[:maxCrashPayloadBytes]
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "crash", "client:"+req.Version, req.Stack)
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

// DevicePolicy returns the upgrade decision for a given device. The
// caller must either own the device (looked up by UserID) or be an
// admin in the same organisation as the device owner; other callers
// receive 403 so canary / forced-upgrade state does not leak across
// users or tenants.
func (h *Handlers) DevicePolicy(c *gin.Context) {
	u := currentUser(c)
	deviceID := c.Query("device_id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id required"})
		return
	}
	devices, _ := h.store.ListDevices(c.Request.Context())
	var found *models.Device
	for i := range devices {
		if devices[i].ID == deviceID {
			found = &devices[i]
			break
		}
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if found.UserID != u.ID {
		if u.Role != models.RoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		owner, err := h.store.UserByID(c.Request.Context(), found.UserID)
		if err != nil || owner.OrgID != u.OrgID {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}
	version := c.Query("version")
	decision := gin.H{"upgrade_required": false, "forced_version": "", "canary": found.Canary}
	if found.ForcedUpgradeTo != "" && found.ForcedUpgradeTo != version {
		decision["upgrade_required"] = true
		decision["forced_version"] = found.ForcedUpgradeTo
	}
	c.JSON(http.StatusOK, decision)
}
