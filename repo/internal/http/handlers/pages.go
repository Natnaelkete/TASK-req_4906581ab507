package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

// Home serves the role-aware landing page with login.
func (h *Handlers) Home(c *gin.Context) {
	html := webtpl.RenderHome()
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// StudentPage serves the student dashboard.
func (h *Handlers) StudentPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	orders, _ := h.store.ListOrdersByStudent(c.Request.Context(), u.ID)
	html := webtpl.RenderStudentDashboard(webtpl.StudentData{User: u, Orders: orders})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// TeacherPage serves the teacher content console. Analytics windows
// are computed from the live content store so the rendered page
// reflects the same numbers as /api/teacher/analytics.
func (h *Handlers) TeacherPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	items, _ := h.store.ListContentByTeacher(c.Request.Context(), u.ID)
	now := time.Now()
	html := webtpl.RenderTeacherDashboard(webtpl.TeacherData{
		User: u,
		Analytics: webtpl.Analytics{
			Window7:  rollupWindow(items, now.Add(-7*24*time.Hour)),
			Window30: rollupWindow(items, now.Add(-30*24*time.Hour)),
			Window90: rollupWindow(items, now.Add(-90*24*time.Hour)),
		},
	})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// rollupWindow accumulates content-item metrics that fall within the
// given trailing window. Matches the rollup in TeacherAnalytics so the
// server-rendered dashboard and the JSON API stay consistent.
func rollupWindow(items []models.ContentItem, since time.Time) webtpl.Window {
	var w webtpl.Window
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

// DispatcherPage serves the dispatcher assignment view. Both the
// delivery queue and the courier roster are scoped to the dispatcher's
// organisation so the HTML surface enforces the same tenant isolation
// as the JSON deliveries API.
func (h *Handlers) DispatcherPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleDispatcher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	allCouriers, _ := h.store.ListUsersByRole(c.Request.Context(), models.RoleCourier)
	couriers := make([]models.User, 0, len(allCouriers))
	for _, co := range allCouriers {
		if co.OrgID == "" || co.OrgID == u.OrgID {
			couriers = append(couriers, co)
		}
	}
	deliveries, _ := h.store.ListDeliveriesByOrg(c.Request.Context(), u.OrgID)
	html := webtpl.RenderDispatcherDashboard(webtpl.DispatcherData{
		User: u, Couriers: couriers, Deliveries: deliveries,
	})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// AdminPage serves the admin console.
func (h *Handlers) AdminPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	html := webtpl.RenderAdminDashboard(webtpl.AdminData{User: u})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
