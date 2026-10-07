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

	"github.com/OurNeZt/ournezt-web/internal/authlimit"
	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/OurNeZt/ournezt-web/internal/web"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type brokenProgressWriter struct{}

func (brokenProgressWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestReviewerCheckStopsWhenProgressOutputFails(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/login?error=Invalid+email+or+password.", http.StatusFound)
	}))
	defer server.Close()
	err := run(context.Background(), settings{baseURL: server.URL, mode: "account", limit: 2}, brokenProgressWriter{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output error was lost: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("check continued after output failure: %d requests", requests.Load())
	}
}

func TestReviewerCheckDetectsMissingThrottling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login?error=not+found", http.StatusFound)
	}))
	defer server.Close()
	err := run(context.Background(), settings{baseURL: server.URL, mode: "account", limit: 2}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "expected 429") {
		t.Fatalf("unfixed endpoint did not fail the check: %v", err)
	}
}

type reviewerAuthClient struct {
	pb.AuthServiceClient
	calls atomic.Int32
}

func (f *reviewerAuthClient) Login(context.Context, *pb.LoginRequest, ...grpc.CallOption) (*pb.LoginResponse, error) {
	f.calls.Add(1)
	return nil, status.Error(codes.Unauthenticated, "unauthenticated")
}

func TestReviewerCheckAgainstApplicationRouter(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, mode := range []string{"account", "ip"} {
		t.Run(mode, func(t *testing.T) {
			client := &reviewerAuthClient{}
			limits := authlimit.Config{IPLimit: 100, AccountLimit: 2}
			if mode == "ip" {
				limits.IPLimit, limits.AccountLimit = 2, 100
			}
			router, err := web.NewRouter(config.Config{AuthLimits: limits}, &core.Clients{Auth: client})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			var out bytes.Buffer
			if err := run(context.Background(), settings{baseURL: server.URL, mode: mode, limit: 2}, &out); err != nil {
				t.Fatal(err)
			}
			if client.calls.Load() != 2 {
				t.Fatalf("throttled requests reached Core: %d calls", client.calls.Load())
			}
		})
	}
}

func TestReviewerCheckRejectsAlreadyThrottledOrUnavailableEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		err := run(context.Background(), settings{baseURL: server.URL, mode: "account", limit: 2}, &bytes.Buffer{})
		server.Close()
		if err == nil {
			t.Fatalf("HTTP %d endpoint falsely passed", status)
		}
	}
}

func TestReviewerCheckAccountAndIPModes(t *testing.T) {
	for _, mode := range []string{"account", "ip"} {
		t.Run(mode, func(t *testing.T) {
			var posts int
			var emails, forwarded []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusOK)
					return
				}
				posts++
				emails = append(emails, r.PostFormValue("email"))
				forwarded = append(forwarded, r.Header.Get("X-Forwarded-For"))
				lastBlocked := 3
				if mode == "account" {
					lastBlocked = 4
				}
				if posts > 2 && posts <= lastBlocked {
					w.Header().Set("Retry-After", "1")
					w.Header().Set("Cache-Control", "no-store")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				http.Redirect(w, r, "/login?error=Invalid+email+or+password.", http.StatusFound)
			}))
			defer server.Close()
			var out bytes.Buffer
			err := run(context.Background(), settings{baseURL: server.URL, mode: mode, limit: 2, recovery: true, maxWait: 3 * time.Second}, &out)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "attempts resume") {
				t.Fatal("recovery not checked")
			}
			if mode == "account" && (emails[0] != emails[1] || strings.ToLower(strings.TrimSpace(emails[3])) != emails[0]) {
				t.Fatal("account normalization not exercised")
			}
			if mode == "ip" && (emails[0] == emails[1] || forwarded[0] == "" || forwarded[0] == forwarded[1]) {
				t.Fatal("email and forwarding headers did not rotate")
			}
		})
	}
}
