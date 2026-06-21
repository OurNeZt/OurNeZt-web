package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type adminUsersData struct {
	Users                   []*ourneztv1.User
	TotalUsers              int
	ActiveUsers             int
	DisabledUsers           int
	AdminUsers              int
	MustChangePasswordUsers int
	CurrentUserID           string
}

type adminDashboardData struct {
	TotalUsers              int
	ActiveUsers             int
	DisabledUsers           int
	AdminUsers              int
	MustChangePasswordUsers int
}

func (a *App) adminHome(c *gin.Context) {
	data, err := a.fetchAdminUsersData(c)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	a.render(c, "admin_dashboard", "Admin Dashboard", adminDashboardData{
		TotalUsers:              data.TotalUsers,
		ActiveUsers:             data.ActiveUsers,
		DisabledUsers:           data.DisabledUsers,
		AdminUsers:              data.AdminUsers,
		MustChangePasswordUsers: data.MustChangePasswordUsers,
	})
}

func (a *App) adminUsers(c *gin.Context) {
	data, err := a.fetchAdminUsersData(c)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	a.render(c, "admin_users", "Admin Users", data)
}

func (a *App) adminCreateUser(c *gin.Context) {
	role := strings.TrimSpace(c.PostForm("role"))
	if role == "" {
		role = "user"
	}

	_, err := a.clients.Auth.CreateUser(a.grpcContext(c), &ourneztv1.CreateUserRequest{
		Email:       strings.TrimSpace(c.PostForm("email")),
		DisplayName: strings.TrimSpace(c.PostForm("display_name")),
		Password:    c.PostForm("password"),
		Role:        role,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	c.Redirect(http.StatusSeeOther, "/admin/users?flash=User+created")
}

func (a *App) adminDisableUser(c *gin.Context) {
	_, err := a.clients.Auth.DisableUser(a.grpcContext(c), &ourneztv1.DisableUserRequest{UserId: c.Param("id")})
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/admin/users?flash=User+disabled")
}

func (a *App) fetchAdminUsersData(c *gin.Context) (adminUsersData, error) {
	resp, err := a.clients.Auth.ListUsers(a.grpcContext(c), &ourneztv1.ListUsersRequest{})
	if err != nil {
		return adminUsersData{}, err
	}

	data := adminUsersData{
		Users:         resp.GetUsers(),
		CurrentUserID: "",
	}

	if user := userFromContext(c); user != nil {
		data.CurrentUserID = user.ID
	}

	data.TotalUsers = len(data.Users)
	for _, u := range data.Users {
		if u.GetRole() == "admin" {
			data.AdminUsers++
		}
		if u.GetDisabled() {
			data.DisabledUsers++
		} else {
			data.ActiveUsers++
		}
		if u.GetMustChangePassword() {
			data.MustChangePasswordUsers++
		}
	}

	return data, nil
}
