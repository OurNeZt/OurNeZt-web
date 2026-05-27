package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type adminUsersData struct {
	JustCreated *ourneztv1.User
}

type adminDashboardData struct{}

func (a *App) adminHome(c *gin.Context) {
	a.render(c, "admin_dashboard", "Admin Dashboard", adminDashboardData{})
}

func (a *App) adminUsers(c *gin.Context) {
	a.render(c, "admin_users", "Admin Users", adminUsersData{})
}

func (a *App) adminCreateUser(c *gin.Context) {
	role := strings.TrimSpace(c.PostForm("role"))
	if role == "" {
		role = "user"
	}

	resp, err := a.clients.Auth.CreateUser(a.grpcContext(c), &ourneztv1.CreateUserRequest{
		Email:       strings.TrimSpace(c.PostForm("email")),
		DisplayName: strings.TrimSpace(c.PostForm("display_name")),
		Password:    c.PostForm("password"),
		Role:        role,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	a.render(c, "admin_users", "Admin Users", adminUsersData{JustCreated: resp})
}

func (a *App) adminDisableUser(c *gin.Context) {
	_, err := a.clients.Auth.DisableUser(a.grpcContext(c), &ourneztv1.DisableUserRequest{UserId: c.Param("id")})
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/admin/users?flash=User+disabled")
}
