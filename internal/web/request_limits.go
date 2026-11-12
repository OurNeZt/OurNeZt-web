package web

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/gin-gonic/gin"
)

const authFormBodyLimit int64 = 16 << 10

func isAuthSubmission(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/login", "/change-password", "/profile/password", "/admin/users":
		return true
	default:
		return false
	}
}

// requestBodyLimit validates the entire bounded body before session loading or
// handlers can use partial form data. Authentication concurrency is acquired
// first, so slow auth submissions still occupy the existing limited capacity.
func (a *App) requestBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := a.cfg.MaxRequestBodyBytes
		if limit <= 0 {
			limit = config.DefaultMaxRequestBodyBytes
		}
		if isAuthSubmission(c.Request) && limit > authFormBodyLimit {
			limit = authFormBodyLimit
		}
		if c.Request.ContentLength > limit {
			rejectRequestBody(c, http.StatusRequestEntityTooLarge, "Request body is too large.")
			return
		}
		if c.Request.Body == nil {
			c.Next()
			return
		}

		// Give MaxBytesReader the underlying HTTP writer so net/http can mark an
		// oversized HTTP/1 request's connection for closure instead of draining it.
		writer := http.ResponseWriter(c.Writer)
		if unwrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter }); ok {
			writer = unwrapper.Unwrap()
		}
		bounded := http.MaxBytesReader(writer, c.Request.Body, limit)
		body, err := io.ReadAll(bounded)
		if err != nil {
			var tooLarge *http.MaxBytesError
			var timeout net.Error
			switch {
			case errors.As(err, &tooLarge):
				rejectRequestBody(c, http.StatusRequestEntityTooLarge, "Request body is too large.")
			case errors.As(err, &timeout) && timeout.Timeout():
				rejectRequestBody(c, http.StatusRequestTimeout, "Request body timed out.")
			default:
				rejectRequestBody(c, http.StatusBadRequest, "Invalid request body.")
			}
			return
		}
		// net/http closes the original stream. Replay only the validated bytes
		// for form parsing and handlers; no unbounded read can follow.
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		if contentType := c.GetHeader("Content-Type"); contentType != "" {
			mediaType, _, parseErr := mime.ParseMediaType(contentType)
			if parseErr == nil {
				switch mediaType {
				case "application/x-www-form-urlencoded":
					parseErr = c.Request.ParseForm()
				case "multipart/form-data":
					// The entire multipart message is already bounded. Keeping its
					// files within that same memory budget avoids temporary-file spills.
					parseErr = c.Request.ParseMultipartForm(limit)
				}
			}
			if c.Request.MultipartForm != nil {
				defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
			}
			if parseErr != nil {
				rejectRequestBody(c, http.StatusBadRequest, "Invalid request body.")
				return
			}
		}
		c.Next()
	}
}

func rejectRequestBody(c *gin.Context, status int, message string) {
	c.Header("Cache-Control", "no-store")
	if c.Request.ProtoMajor == 1 {
		c.Header("Connection", "close")
		// Stop draining unread bytes during HTTP/1 connection cleanup.
		// HTTP/2 closes the request stream when the handler returns instead.
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now())
	}
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
