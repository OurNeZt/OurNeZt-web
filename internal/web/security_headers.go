package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const cspNonceKey = "csp_nonce"

// Run before middleware that can render errors, so every response gets the
// policy and every rendered page can authorize only its own script elements.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		var nonceBytes [32]byte
		if _, err := rand.Read(nonceBytes[:]); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		nonce := base64.StdEncoding.EncodeToString(nonceBytes[:])
		c.Set(cspNonceKey, nonce)
		c.Header("Content-Security-Policy", "default-src 'none'; "+
			"script-src 'nonce-"+nonce+"'; script-src-attr 'none'; "+
			"style-src 'self'; style-src-attr 'none'; img-src 'self' data:; "+
			"font-src 'self'; connect-src 'self'; form-action 'self'; "+
			"base-uri 'none'; frame-ancestors 'none'; object-src 'none'")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		// Only the actual connection establishes TLS here. A TLS-terminating
		// ingress owns HSTS on public HTTPS responses; forwarded headers and
		// secure-cookie settings are not evidence of an encrypted connection.
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000")
		}
		// Never cache HTML containing a per-response nonce, including early errors.
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || !strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}
