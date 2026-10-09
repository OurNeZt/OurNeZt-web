package web

import (
	"context"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

const authRetryMessage = "Too many authentication attempts. Please try again later."

// authTrafficLimit runs before session loading and form parsing. Expensive
// authentication routes share an IP budget and an in-flight RPC budget.
func (a *App) authTrafficLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/login", "/change-password", "/profile/password", "/admin/users":
		default:
			c.Next()
			return
		}
		ip := c.ClientIP()
		if addr, err := netip.ParseAddr(ip); err == nil {
			ip = addr.Unmap().WithZone("").String()
		}
		if retry := a.authLimits.AllowIP(ip); retry > 0 {
			rejectAuthTraffic(c, retry)
			return
		}
		release, err := a.authLimits.Acquire(c.Request.Context())
		if err != nil {
			rejectAuthTraffic(c, time.Second)
			return
		}
		defer release()
		// Limit these small forms before parsing attacker-controlled account keys.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
		if err := c.Request.ParseForm(); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid authentication request."})
			return
		}
		timeout := a.cfg.RequestTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Both password-change URLs use the same user bucket, preventing route hopping.
func (a *App) authAccountLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		var account string
		switch c.Request.URL.Path {
		case "/login":
			account = "login:" + strings.ToLower(strings.TrimSpace(c.PostForm("email")))
		case "/admin/users":
			if user := userFromContext(c); user != nil {
				account = "create:" + user.ID
			}
		default:
			if user := userFromContext(c); user != nil {
				account = "password:" + user.ID
			}
		}
		if retry := a.authLimits.AllowAccount(account); retry > 0 {
			rejectAuthTraffic(c, retry)
			return
		}
		c.Next()
	}
}

func rejectAuthTraffic(c *gin.Context, retry time.Duration) {
	seconds := int64(retry / time.Second)
	if retry%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	c.Header("Cache-Control", "no-store")
	c.Abort()
	data := ViewData{Title: "Please try again later", User: userFromContext(c), Error: authRetryMessage}
	if c.Request.URL.Path == "/login" {
		data.Title, data.Data = "Login", loginPayload{}
		c.HTML(http.StatusTooManyRequests, "login", data)
		return
	}
	returnURL := "/profile"
	if c.Request.URL.Path == "/admin/users" || c.Request.URL.Path == "/change-password" {
		returnURL = c.Request.URL.Path
	}
	data.Data = struct{ ReturnURL string }{ReturnURL: returnURL}
	c.HTML(http.StatusTooManyRequests, "auth_throttled", data)
}

func authRetryDelay(err error) time.Duration {
	for _, detail := range status.Convert(err).Details() {
		if info, ok := detail.(*errdetails.RetryInfo); ok && info.GetRetryDelay().IsValid() {
			if retry := info.GetRetryDelay().AsDuration(); retry > 0 {
				return retry
			}
		}
	}
	return time.Minute
}
