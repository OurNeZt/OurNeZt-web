package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/OurNeZt/ournezt-web/internal/web"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type reviewerAuthClient struct {
	pb.AuthServiceClient
	mu       sync.Mutex
	requests []*pb.LoginRequest
}

func (c *reviewerAuthClient) Login(_ context.Context, req *pb.LoginRequest, _ ...grpc.CallOption) (*pb.LoginResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	code := codes.NotFound
	if req.GetEmail() == "active@example.com" {
		code = codes.Unauthenticated
	} else if req.GetEmail() == "disabled@example.com" {
		code = codes.PermissionDenied
	}
	return nil, status.Error(code, "private account detail")
}

func TestReviewerCheckAgainstApplicationRouter(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &reviewerAuthClient{}
	router, err := web.NewRouter(config.Config{}, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	var out bytes.Buffer
	err = run(context.Background(), settings{baseURL: server.URL, email: " Active@Example.COM ", disabledEmail: "disabled@example.com", timeout: time.Second}, &out)
	if err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.requests) != 3 || !strings.HasSuffix(client.requests[0].GetEmail(), "@example.invalid") || client.requests[1].GetEmail() != "active@example.com" || client.requests[2].GetEmail() != "disabled@example.com" {
		t.Fatalf("missing fixture coverage: %v", client.requests)
	}
	for _, req := range client.requests {
		if !strings.HasPrefix(req.GetPassword(), "deliberately-wrong-") || req.GetPassword() != client.requests[0].GetPassword() {
			t.Fatal("probes must use the same generated wrong password")
		}
	}
	if strings.Count(out.String(), "PASS:") != 4 || strings.Contains(out.String(), "private") || strings.Contains(out.String(), "SKIP") {
		t.Fatalf("unexpected reviewer output: %s", &out)
	}
}

func TestReviewerCheckDetectsUnsafeOrIncompleteResponses(t *testing.T) {
	for _, mode := range []string{"raw message", "different body", "cookie", "throttle", "outage", "success", "missing rendered message", "external redirect", "oversized page"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "text/html")
					page := "<html>Invalid email or password.</html>"
					switch mode {
					case "missing rendered message":
						page = "<html>private account detail</html>"
					case "oversized page":
						page = strings.Repeat("x", (2<<20)+1)
					}
					_, _ = io.WriteString(w, page)
					return
				}
				location := "/login?error=Invalid+email+or+password."
				switch mode {
				case "raw message":
					location = "/login?error=not+found"
				case "external redirect":
					location = "https://example.invalid" + location
				case "cookie":
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "token"})
				case "throttle":
					w.WriteHeader(http.StatusTooManyRequests)
					return
				case "outage":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				case "success":
					location = "/dashboard?flash=Welcome"
				}
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
				if mode == "different body" {
					_, _ = io.WriteString(w, r.PostFormValue("email"))
				}
			}))
			defer server.Close()
			if err := run(context.Background(), settings{baseURL: server.URL, email: "active@example.com", timeout: time.Second}, &bytes.Buffer{}); err == nil {
				t.Fatalf("%s endpoint falsely passed", mode)
			}
		})
	}
}

func TestReviewerCheckReportsSkippedDisabledFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Redirect(w, r, "/login?error=Invalid+email+or+password.", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>Invalid email or password.</html>")
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := run(context.Background(), settings{baseURL: server.URL, email: "active@example.com", timeout: time.Second}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "SKIP: disabled account") {
		t.Fatal("omitted disabled fixture was not reported")
	}
}

func TestReviewerCheckRejectsInvalidSettings(t *testing.T) {
	for _, cfg := range []settings{
		{baseURL: "https://user:secret@example.invalid", email: "active@example.com", timeout: time.Second},
		{baseURL: "http://localhost:8080/path", email: "active@example.com", timeout: time.Second},
		{baseURL: "http://localhost:8080", timeout: time.Second},
		{baseURL: "http://localhost:8080", email: "active@example.com"},
		{baseURL: "http://localhost:8080", email: "active@example.com", disabledEmail: " ACTIVE@example.com ", timeout: time.Second},
	} {
		if err := run(context.Background(), cfg, &bytes.Buffer{}); err == nil {
			t.Fatal("invalid settings were accepted")
		}
	}
}
