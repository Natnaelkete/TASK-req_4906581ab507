package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// TeacherProfile returns the public creator profile fields for the
// authenticated teacher, plus their pinned featured works loaded from
// the content store.
func (h *Handlers) TeacherProfile(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	items, _ := h.store.ListContentByTeacher(c.Request.Context(), u.ID)
	pinned := []gin.H{}
	for _, it := range items {
		if it.Pinned {
			pinned = append(pinned, gin.H{"id": it.ID, "title": it.Title})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"username": u.Username,
		"display":  u.DisplayName,
		"rating":   u.Rating,
		"pinned":   pinned,
		"total":    len(items),
	})
}

// TeacherPin toggles the Pinned flag on a single content item owned
// by the authenticated teacher.
func (h *Handlers) TeacherPin(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		ContentID string `json:"content_id"`
		Pinned    bool   `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ContentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	item, err := h.store.ContentByID(c.Request.Context(), req.ContentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	if item.TeacherID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your content"})
		return
	}
	item.Pinned = req.Pinned
	item.UpdatedAt = time.Now()
	if err := h.store.UpsertContent(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "teacher.pin", "content:"+item.ID, fmt.Sprintf("pinned=%v", req.Pinned))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TeacherBulk applies a bulk action (unpublish, delete, edit) to a list
// of content ids owned by the authenticated teacher. Foreign-owned
// items are rejected even if their ids are supplied.
func (h *Handlers) TeacherBulk(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
		Title  string   `json:"title"` // for edit
		Body   string   `json:"body"`  // for edit
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	applied := 0
	for _, id := range req.IDs {
		item, err := h.store.ContentByID(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if item.TeacherID != u.ID {
			continue // silently skip foreign items
		}
		switch req.Action {
		case "delete":
			_ = h.store.DeleteContent(c.Request.Context(), item.ID)
		case "unpublish":
			item.Published = false
			item.UpdatedAt = time.Now()
			_ = h.store.UpsertContent(c.Request.Context(), item)
		case "publish":
			item.Published = true
			item.UpdatedAt = time.Now()
			_ = h.store.UpsertContent(c.Request.Context(), item)
		case "edit":
			if req.Title != "" {
				item.Title = req.Title
			}
			if req.Body != "" {
				item.Body = req.Body
			}
			item.UpdatedAt = time.Now()
			_ = h.store.UpsertContent(c.Request.Context(), item)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported action"})
			return
		}
		applied++
		_, _ = h.chain.Append(c.Request.Context(), u.OrgID, u.Username, "teacher.bulk."+req.Action, "content:"+item.ID, "")
	}
	c.JSON(http.StatusOK, gin.H{"applied": applied, "action": req.Action})
}

// TeacherAnalytics returns 7/30/90-day roll-ups computed from the
// content store. Items without a CreatedAt inside a window contribute
// to that window if they were published before the window started —
// that's the canonical blog-style metric (cumulative over the window).
func (h *Handlers) TeacherAnalytics(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	items, _ := h.store.ListContentByTeacher(c.Request.Context(), u.ID)
	now := time.Now()
	c.JSON(http.StatusOK, gin.H{
		"window_7d":  rollup(items, now.Add(-7*24*time.Hour)),
		"window_30d": rollup(items, now.Add(-30*24*time.Hour)),
		"window_90d": rollup(items, now.Add(-90*24*time.Hour)),
		"as_of":      now,
		"total":      len(items),
	})
}

type window struct {
	Views     int `json:"views"`
	Likes     int `json:"likes"`
	Favorites int `json:"favorites"`
	Followers int `json:"followers"`
}

func rollup(items []models.ContentItem, since time.Time) window {
	var w window
	for _, it := range items {
		if it.CreatedAt.Before(since) {
			continue
		}
		w.Views += it.Views
		w.Likes += it.Likes
		w.Favorites += it.Favorites
		w.Followers += it.Followers
	}
	return w
}
