package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/http/middleware"
	"github.com/eaglepoint/harborclass/internal/models"
)

// Login exchanges username + password for a bearer token.
func (h *Handlers) Login(c *gin.Context) {
	var req struct{ Username, Password string }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	tok, u, err := h.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "login", "auth", "")
	c.JSON(http.StatusOK, gin.H{
		"token":       tok,
		"role":        u.Role,
		"username":    u.Username,
		"display":     u.DisplayName,
		"org_id":      u.OrgID,
		"phone_mask":  auth.MaskPhone(u.DisplayName), // display placeholder; real phone stays ciphered
	})
}

// Logout invalidates the supplied token.
func (h *Handlers) Logout(c *gin.Context) {
	tok := auth.ExtractBearerToken(c.GetHeader("Authorization"))
	if tok == "" {
		tok = c.GetHeader("X-Api-Token")
	}
	h.auth.Logout(tok)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// WhoAmI returns the authenticated user's safe profile (PII masked).
func (h *Handlers) WhoAmI(c *gin.Context) {
	u := currentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"username":    u.Username,
		"role":        u.Role,
		"display":     u.DisplayName,
		"org_id":      u.OrgID,
		"phone_mask":  auth.MaskPhone(""), // real phone lives encrypted only
		"class_ids":   u.ClassIDs,
	})
}

func currentUser(c *gin.Context) models.User {
	v, ok := c.Get(string(middleware.UserKey))
	if !ok {
		return models.User{}
	}
	u, _ := v.(models.User)
	return u
}
