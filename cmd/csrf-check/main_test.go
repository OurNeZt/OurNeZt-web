package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	"github.com/OurNeZt/ournezt-web/internal/web"
)

func TestCSRFReviewerAgainstApplication(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "HTTPS"}[tls], func(t *testing.T) {
			// Any accidental backend call would panic: the probes must stay
			// signed out and use only empty login forms for positive controls.
			router, err := web.NewRouter(config.Config{CookieSecure: tls}, &core.Clients{})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(router)
			if tls {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			var out bytes.Buffer
			if err := run(context.Background(), settings{baseURL: server.URL, timeout: time.Second, transport: server.Client().Transport}, &out); err != nil {
				t.Fatal(err)
			}
			if strings.Count(out.String(), "PASS:") != 11 || !strings.Contains(out.String(), "SKIP: authenticated") {
				t.Fatalf("reviewer coverage was not clearly reported: %s", &out)
			}
			covered := make(map[string]bool)
			for _, group := range postGroups {
				for _, path := range group.routes {
					if covered[path] {
						t.Fatalf("duplicate reviewer route: %s", path)
					}
					covered[path] = true
				}
			}
			for _, route := range router.Routes() {
				if route.Method == http.MethodPost {
					if !covered[route.Path] {
						t.Errorf("reviewer command misses POST %s", route.Path)
					}
					delete(covered, route.Path)
				}
			}
			if len(covered) != 0 {
				t.Errorf("reviewer probes unregistered routes: %v", covered)
			}
		})
	}
}

func TestCSRFReviewerDetectsBrokenProtection(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, mode := range []string{"always reject", "no protection", "foreign origin accepted", "foreign referer accepted", "missing housing protection", "outage", "throttle", "cacheable rejection", "application cookie", "oversized page"} {
		t.Run(mode, func(t *testing.T) {
			router, err := web.NewRouter(config.Config{}, &core.Clients{})
			if err != nil {
				t.Fatal(err)
			}
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if mode == "application cookie" {
						http.SetCookie(w, &http.Cookie{Name: "session", Value: "must-not-be-used"})
					}
					if mode == "oversized page" {
						_, _ = io.WriteString(w, strings.Repeat("x", (2<<20)+1))
						return
					}
					router.ServeHTTP(w, r)
					return
				}
				posts.Add(1)
				w.Header().Set("Cache-Control", "no-store")
				switch {
				case mode == "always reject":
					w.WriteHeader(http.StatusForbidden)
				case mode == "cacheable rejection":
					w.Header().Del("Cache-Control")
					w.WriteHeader(http.StatusForbidden)
				case mode == "no protection", mode == "foreign origin accepted" && strings.Contains(r.Header.Get("Origin"), "csrf-review.invalid"), mode == "foreign referer accepted" && strings.Contains(r.Referer(), "csrf-review.invalid"), mode == "missing housing protection" && r.URL.Path == "/housing/groups":
					http.Redirect(w, r, "/login?error=Invalid+email+or+password.", http.StatusFound)
				case mode == "outage":
					w.WriteHeader(http.StatusServiceUnavailable)
				case mode == "throttle":
					w.WriteHeader(http.StatusTooManyRequests)
				default:
					router.ServeHTTP(w, r)
				}
			}))
			defer server.Close()
			if err := run(context.Background(), settings{baseURL: server.URL, timeout: time.Second}, &bytes.Buffer{}); err == nil {
				t.Fatalf("%s endpoint falsely passed", mode)
			}
			if mode == "application cookie" && posts.Load() != 0 {
				t.Fatal("reviewer continued after receiving an application session cookie")
			}
		})
	}
}

type brokenProgressWriter struct{}

func (brokenProgressWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCSRFReviewerStopsOnOutputError(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	router, err := web.NewRouter(config.Config{}, &core.Clients{})
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	err = run(context.Background(), settings{baseURL: server.URL, timeout: time.Second}, brokenProgressWriter{})
	if !errors.Is(err, io.ErrClosedPipe) || requests.Load() != 2 {
		t.Fatalf("output failure did not stop probes: %v; requests=%d", err, requests.Load())
	}
}

func TestCSRFReviewerValidatesSettings(t *testing.T) {
	for _, cfg := range []settings{
		{baseURL: "file:///tmp/web", timeout: time.Second},
		{baseURL: "https://user:password@example.invalid", timeout: time.Second},
		{baseURL: "http://localhost/path", timeout: time.Second},
		{baseURL: "http://localhost?query=1", timeout: time.Second},
		{baseURL: "http://localhost#fragment", timeout: time.Second},
		{baseURL: "http://localhost", timeout: 0},
	} {
		if err := run(context.Background(), cfg, &bytes.Buffer{}); err == nil {
			t.Fatal("invalid settings accepted")
		}
	}
}
