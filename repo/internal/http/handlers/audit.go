package handlers

import (
	"bytes"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// SearchAudit searches the audit log by actor/resource/time range.
func (h *Handlers) SearchAudit(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	filter := parseAuditFilter(c)
	rows, err := h.chain.Search(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows, "count": len(rows)})
}

// ExportAudit streams filtered audit rows as CSV for local save.
func (h *Handlers) ExportAudit(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	filter := parseAuditFilter(c)
	var buf bytes.Buffer
	if err := h.chain.ExportTo(c.Request.Context(), filter, &buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "audit.export", "audit_log", "")
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
