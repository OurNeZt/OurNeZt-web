package web

import (
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type loginPayload struct{}
type changePasswordPayload struct{}
type bootstrapAdminHelpPayload struct{}

func (a *App) home(c *gin.Context) {
	user := userFromContext(c)
	if user != nil {
		if user.MustChangePassword {
			c.Redirect(http.StatusFound, "/change-password")
			return
		}
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.Redirect(http.StatusFound, "/login")
}

func (a *App) showLogin(c *gin.Context) {
	user := userFromContext(c)
	if user != nil {
		if user.MustChangePassword {
			c.Redirect(http.StatusFound, "/change-password")
			return
		}
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	a.render(c, "login", "Login", loginPayload{})
}

func (a *App) bootstrapAdminHelp(c *gin.Context) {
	a.render(c, "bootstrap_admin_help", "Bootstrap Admin Help", bootstrapAdminHelpPayload{})
}

func (a *App) login(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	if email == "" || password == "" {
		c.Redirect(http.StatusFound, "/login?error=Email+and+password+are+required")
		return
	}

	resp, err := a.clients.Auth.Login(a.grpcContext(c), &ourneztv1.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	http.SetCookie(c.Writer, sessionCookie(a.cfg.SessionCookieName, resp.GetSessionToken(), a.cfg.SessionCookieMax, a.cfg.CookieSecure))
	if resp.GetUser().GetMustChangePassword() {
		c.Redirect(http.StatusFound, "/change-password?flash=Please+set+a+new+password")
		return
	}
	c.Redirect(http.StatusFound, "/dashboard?flash=Welcome")
}

func (a *App) logout(c *gin.Context) {
	http.SetCookie(c.Writer, clearSessionCookie(a.cfg.SessionCookieName, a.cfg.CookieSecure))
	c.Redirect(http.StatusFound, "/login?flash=Logged+out")
}

func (a *App) settings(c *gin.Context) {
	a.render(c, "settings", "Settings", nil)
}

func (a *App) showChangePassword(c *gin.Context) {
	if userFromContext(c) == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}
	a.render(c, "change_password", "Change Password", changePasswordPayload{})
}

func (a *App) changePassword(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	currentPassword := strings.TrimSpace(c.PostForm("current_password"))
	newPassword := strings.TrimSpace(c.PostForm("new_password"))
	confirmPassword := strings.TrimSpace(c.PostForm("confirm_new_password"))
	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		c.Redirect(http.StatusFound, "/change-password?error=All+password+fields+are+required")
		return
	}
	if newPassword != confirmPassword {
		c.Redirect(http.StatusFound, "/change-password?error=New+password+confirmation+does+not+match")
		return
	}

	_, err := a.clients.Auth.ChangePassword(a.grpcContext(c), &ourneztv1.ChangePasswordRequest{
		CurrentPassword: currentPassword,
		NewPassword:     newPassword,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/change-password?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	c.Redirect(http.StatusFound, "/dashboard?flash=Password+updated")
}
