package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

const csrfFieldName = "csrf_token"

type csrfContextKey struct{}

// csrfProtection runs after bounded body parsing and before any Core RPC.
// The signed cookie prevents attackers from supplying their own token/cookie
// pair; masked form tokens remain usable across tabs without exposing the seed.
func (a *App) csrfProtection() (gin.HandlerFunc, error) {
	key, err := hex.DecodeString(a.cfg.CSRFAuthKey)
	production := strings.EqualFold(a.cfg.AppEnv, "production")
	if a.cfg.CSRFAuthKey == "" && !production {
		key = make([]byte, 32)
		_, err = rand.Read(key)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("CSRF_AUTH_KEY must contain a 32-byte secret encoded as 64 hexadecimal characters")
	}
	secure := production || a.cfg.CookieSecure
	deny := func(c *gin.Context) {
		c.Abort()
		c.HTML(http.StatusForbidden, "csrf_error", ViewData{Title: "Please reload the form"})
	}
	cookieName := "ournezt_csrf"
	if secure {
		// Browsers prohibit Domain cookies with this prefix, including those
		// injected by a sibling subdomain.
		cookieName = "__Host-ournezt_csrf"
	}
	protect := csrf.Protect(key,
		csrf.FieldName(csrfFieldName), csrf.CookieName(cookieName),
		csrf.Path("/"), csrf.Secure(secure), csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := r.Context().Value(csrfContextKey{}).(*gin.Context)
			c.Request = r
			deny(c)
		})),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(csrfContextKey{}).(*gin.Context)
		c.Request = r
		c.Next()
	}))
	return func(c *gin.Context) {
		if (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) && strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		// Also validate Referer on local HTTP when Origin is absent. Gorilla
		// requires this fallback on HTTPS; this extends it to both transports.
		if !isSafeMethod(c.Request.Method) && c.GetHeader("Origin") == "" && c.GetHeader("Referer") != "" {
			scheme := "http"
			if secure || c.Request.TLS != nil {
				scheme = "https"
			}
			ref, err := url.Parse(c.GetHeader("Referer"))
			if err != nil || ref.Scheme != scheme || !strings.EqualFold(ref.Host, c.Request.Host) || ref.User != nil {
				deny(c)
				return
			}
		}
		r := c.Request.WithContext(context.WithValue(c.Request.Context(), csrfContextKey{}, c))
		// Production may terminate TLS at ingress. Never trust arbitrary
		// forwarding headers to downgrade the origin/Referer checks.
		if !secure && r.TLS == nil {
			r = csrf.PlaintextHTTPRequest(r)
		}
		protect.ServeHTTP(c.Writer, r)
		c.Abort()
	}, nil
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
}
