package handlers

import (
	"bytes"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/store"
)

// SearchAudit searches the audit log by actor/resource/time range.
// Authorisation goes through h.can so admins can grant the action to
// other roles via the permission overlay (auth.ActionSearchAudit).
// The filter is always pinned to the caller's OrgID so cross-tenant
// rows are never returned — admins are strictly org-scoped.
func (h *Handlers) SearchAudit(c *gin.Context) {
	u := currentUser(c)
	if !h.can(c, u, auth.ActionSearchAudit, auth.Target{OrgID: u.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	filter := parseAuditFilter(c)
	filter.OrgID = u.OrgID
	rows, err := h.chain.Search(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows, "count": len(rows)})
}

// ExportAudit streams filtered audit rows as CSV for local save.
// Authorisation goes through h.can so admins can grant the action to
// other roles via the permission overlay (auth.ActionExportAudit).
// Same OrgID pinning as SearchAudit applies so exports never contain
// another organisation's entries.
func (h *Handlers) ExportAudit(c *gin.Context) {
	u := currentUser(c)
	if !h.can(c, u, auth.ActionExportAudit, auth.Target{OrgID: u.OrgID}) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	filter := parseAuditFilter(c)
	filter.OrgID = u.OrgID
	var buf bytes.Buffer
	if err := h.chain.ExportTo(c.Request.Context(), filter, &buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "audit.export", "audit_log", "")
	c.Header("Content-Disposition", `attachment; filename="audit.csv"`)
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

func parseAuditFilter(c *gin.Context) store.AuditFilter {
	f := store.AuditFilter{
		Actor:    c.Query("actor"),
		Resource: c.Query("resource"),
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = t
		}
	}
	return f
}
