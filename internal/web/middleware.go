package web

import (
	"context"
	"net/http"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
)

func (a *App) sessionLoader() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Request.Cookie(a.cfg.SessionCookieName)
		if err != nil || cookie.Value == "" {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), a.cfg.RequestTimeout)
		defer cancel()
		resp, rpcErr := a.clients.Auth.ValidateSession(ctx, &ourneztv1.ValidateSessionRequest{SessionToken: cookie.Value})
		if rpcErr != nil {
			if grpcCode(rpcErr) == codes.Unauthenticated || grpcCode(rpcErr) == codes.PermissionDenied {
				http.SetCookie(c.Writer, clearSessionCookie(a.cfg.SessionCookieName, a.cfg.CookieSecure))
			}
			c.Next()
			return
		}

		user := userFromProto(resp.GetUser())
		if user != nil {
			c.Set(userContextKey, user)
			c.Set(tokenContextKey, cookie.Value)
		}

		c.Next()
	}
}

func (a *App) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if userFromContext(c) == nil {
			c.Redirect(http.StatusFound, "/login?error=Please+log+in")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *App) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := userFromContext(c)
		if user == nil || user.Role != "admin" {
			c.Redirect(http.StatusFound, "/dashboard?error=Admin+access+required")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *App) requirePasswordChanged() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := userFromContext(c)
		if user == nil || !user.MustChangePassword {
			c.Next()
			return
		}

		c.Redirect(http.StatusFound, "/change-password?error=Password+change+required")
		c.Abort()
	}
}

func (a *App) requireStandardUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := userFromContext(c)
		if user == nil {
			c.Redirect(http.StatusFound, "/login?error=Please+log+in")
			c.Abort()
			return
		}
		if user.Role == "admin" {
			c.Redirect(http.StatusFound, "/admin?error=This+section+is+for+non-admin+users")
			c.Abort()
			return
		}
		c.Next()
	}
}
