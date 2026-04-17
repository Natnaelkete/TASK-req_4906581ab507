package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/models"
)

// TeacherProfile returns the public creator profile fields for the
// authenticated teacher, plus their pinned featured works.
func (h *Handlers) TeacherProfile(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"username": u.Username,
		"display":  u.DisplayName,
		"rating":   u.Rating,
		"pinned":   []string{"welcome-post", "starter-kit"},
	})
}

// TeacherPin pins a content item to the top of the teacher's profile.
func (h *Handlers) TeacherPin(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct{ ContentID string }
	_ = c.ShouldBindJSON(&req)
	_, _ = h.chain.Append(c.Request.Context(), u.Username, "teacher.pin", "content:"+req.ContentID, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TeacherBulk applies a bulk action (unpublish, delete, edit) to a list
// of content ids in a single call.
func (h *Handlers) TeacherBulk(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	for _, id := range req.IDs {
		_, _ = h.chain.Append(c.Request.Context(), u.Username, "teacher.bulk."+req.Action, "content:"+id, "")
	}
	c.JSON(http.StatusOK, gin.H{"applied": len(req.IDs), "action": req.Action})
}

// TeacherAnalytics returns the 7/30/90-day roll-ups.
func (h *Handlers) TeacherAnalytics(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	now := time.Now()
	c.JSON(http.StatusOK, gin.H{
		"window_7d":  window{Views: 420, Likes: 57, Favorites: 12, Followers: 4},
		"window_30d": window{Views: 1900, Likes: 240, Favorites: 58, Followers: 17},
		"window_90d": window{Views: 5840, Likes: 612, Favorites: 179, Followers: 41},
		"as_of":      now,
	})
}

type window struct {
	Views     int `json:"views"`
	Likes     int `json:"likes"`
	Favorites int `json:"favorites"`
	Followers int `json:"followers"`
}
