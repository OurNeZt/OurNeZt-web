package web

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

func bodyLimitRouter(cfg config.Config, handler gin.HandlerFunc) *gin.Engine {
	app := &App{cfg: cfg}
	r := gin.New()
	r.Use(app.requestBodyLimit())
	r.Any("/submit", handler)
	r.Any("/login", handler)
	return r
}

func TestRequestBodyLimits(t *testing.T) {
	for _, unknownLength := range []bool{false, true} {
		for _, path := range []string{"/submit", "/login", "/missing"} {
			t.Run(path+map[bool]string{false: " known length", true: " unknown length"}[unknownLength], func(t *testing.T) {
				called := false
				r := bodyLimitRouter(config.Config{MaxRequestBodyBytes: 64}, func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("a", 65)))
				if unknownLength {
					req.ContentLength = -1
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != http.StatusRequestEntityTooLarge || called || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("oversized body was not rejected before handling: %d, called=%t", w.Code, called)
				}
			})
		}
	}
	t.Run("exact limit preserves body", func(t *testing.T) {
		payload := strings.Repeat("x", 64)
		r := bodyLimitRouter(config.Config{MaxRequestBodyBytes: 64}, func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			if err != nil || string(body) != payload {
				t.Errorf("validated body was not replayed: %q, %v", body, err)
			}
			c.Status(http.StatusNoContent)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(payload)))
		if w.Code != http.StatusNoContent {
			t.Fatalf("body at limit rejected: %d", w.Code)
		}
	})
	t.Run("auth retains smaller cap", func(t *testing.T) {
		called := false
		r := bodyLimitRouter(config.Config{MaxRequestBodyBytes: 1 << 20}, func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })
		for _, size := range []int{16 << 10, (16 << 10) + 1} {
			called = false
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(strings.Repeat("x", size))))
			if size == 16<<10 && (w.Code != http.StatusNoContent || !called) {
				t.Fatal("auth body at limit rejected")
			}
			if size > 16<<10 && (w.Code != http.StatusRequestEntityTooLarge || called) {
				t.Fatal("auth cap bypassed")
			}
		}
	})
	t.Run("empty requests pass", func(t *testing.T) {
		r := bodyLimitRouter(config.Config{}, func(c *gin.Context) { c.Status(http.StatusNoContent) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/submit", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("empty GET rejected: %d", w.Code)
		}
	})
	t.Run("malformed forms never reach handler", func(t *testing.T) {
		for _, tc := range []struct{ contentType, body string }{
			{"application/x-www-form-urlencoded", "name=valid&bad=%zz"},
			{"multipart/form-data; boundary=missing", "not a multipart form"},
			{"multipart/form-data", "missing boundary"},
		} {
			called := false
			r := bodyLimitRouter(config.Config{}, func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest || called {
				t.Fatalf("malformed form processed: %s, status=%d", tc.contentType, w.Code)
			}
		}
	})
}

func TestBoundedMultipartForms(t *testing.T) {
	tempDir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, tempDir)
	}
	makeMultipart := func(fileSize int) (*bytes.Buffer, string) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("name", "Home viewing"); err != nil {
			t.Fatal(err)
		}
		file, err := writer.CreateFormFile("extra", "unused.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(file, strings.NewReader(strings.Repeat("x", fileSize))); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return &body, writer.FormDataContentType()
	}
	for _, fileSize := range []int{128 << 10, 2 << 20} {
		called := false
		r := bodyLimitRouter(config.Config{}, func(c *gin.Context) {
			called = true
			if c.PostForm("name") != "Home viewing" {
				t.Error("multipart field lost")
			}
			c.Status(http.StatusNoContent)
		})
		body, contentType := makeMultipart(fileSize)
		req := httptest.NewRequest(http.MethodPost, "/submit", body)
		req.ContentLength = -1
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if fileSize < 1<<20 && (w.Code != http.StatusNoContent || !called) {
			t.Fatal("valid multipart form rejected")
		}
		if fileSize > 1<<20 && (w.Code != http.StatusRequestEntityTooLarge || called) {
			t.Fatal("oversized multipart form processed")
		}
		files, err := os.ReadDir(tempDir)
		if err != nil || len(files) != 0 {
			t.Fatalf("multipart request left temporary files: %v, %v", files, err)
		}
	}
}

type requestLimitAuthClient struct {
	limitedAuthClient
	sessions atomic.Int32
}

func (f *requestLimitAuthClient) ValidateSession(context.Context, *pb.ValidateSessionRequest, ...grpc.CallOption) (*pb.ValidateSessionResponse, error) {
	f.sessions.Add(1)
	return &pb.ValidateSessionResponse{}, nil
}

func TestRouterRejectsBodiesBeforeCore(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &requestLimitAuthClient{}
	r, err := NewRouter(config.Config{SessionCookieName: "session", RequestTimeout: time.Second}, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/housing/home/notes", "/families", "/logout", "/login", "/change-password", "/profile/password", "/admin/users"} {
		limit := 1 << 20
		if path == "/login" || path == "/change-password" || path == "/profile/password" || path == "/admin/users" {
			limit = 16 << 10
		}
		body := "name=valid&padding=" + strings.Repeat("x", limit)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.ContentLength = -1
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s returned %d instead of 413", path, w.Code)
		}
	}
	if client.sessions.Load() != 0 || client.logins.Load() != 0 || client.creates.Load() != 0 || client.changes.Load() != 0 {
		t.Fatal("oversized request reached Core")
	}
}

func TestMaximumHousingNotesRemainUsable(t *testing.T) {
	notes := strings.Repeat("🏡", 10000)
	body := url.Values{"notes": {notes}}.Encode()
	r := bodyLimitRouter(config.Config{}, func(c *gin.Context) {
		if c.PostForm("notes") != notes {
			t.Error("large valid note was truncated")
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("valid 10,000-character notes rejected: %d", w.Code)
	}
}
