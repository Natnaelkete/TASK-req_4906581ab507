package handlers

import (
	"net/http"

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

// TeacherPage serves the teacher content console.
func (h *Handlers) TeacherPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	html := webtpl.RenderTeacherDashboard(webtpl.TeacherData{
		User: u,
		Analytics: webtpl.Analytics{
			Window7:  webtpl.Window{Views: 420, Likes: 57, Favorites: 12, Followers: 4},
			Window30: webtpl.Window{Views: 1900, Likes: 240, Favorites: 58, Followers: 17},
			Window90: webtpl.Window{Views: 5840, Likes: 612, Favorites: 179, Followers: 41},
		},
	})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// DispatcherPage serves the dispatcher assignment view.
func (h *Handlers) DispatcherPage(c *gin.Context) {
	u := currentUser(c)
	if u.Role != models.RoleDispatcher {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	couriers, _ := h.store.ListUsersByRole(c.Request.Context(), models.RoleCourier)
	deliveries, _ := h.store.ListDeliveries(c.Request.Context())
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
