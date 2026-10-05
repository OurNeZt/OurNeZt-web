package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/authlimit"
	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type limitedAuthClient struct {
	pb.AuthServiceClient
	logins  atomic.Int32
	changes atomic.Int32
	creates atomic.Int32
	err     error
	user    *pb.User
	entered chan struct{}
	resume  chan struct{}
}

func (f *limitedAuthClient) Login(ctx context.Context, _ *pb.LoginRequest, _ ...grpc.CallOption) (*pb.LoginResponse, error) {
	f.logins.Add(1)
	if _, ok := ctx.Deadline(); !ok {
		return nil, status.Error(codes.Internal, "missing deadline")
	}
	if f.entered != nil {
		close(f.entered)
		<-f.resume
	}
	return &pb.LoginResponse{User: &pb.User{Id: "user"}, SessionToken: "session"}, f.err
}
func (f *limitedAuthClient) ValidateSession(context.Context, *pb.ValidateSessionRequest, ...grpc.CallOption) (*pb.ValidateSessionResponse, error) {
	return &pb.ValidateSessionResponse{User: f.user}, nil
}
func (f *limitedAuthClient) ChangePassword(context.Context, *pb.ChangePasswordRequest, ...grpc.CallOption) (*pb.ChangePasswordResponse, error) {
	f.changes.Add(1)
	return &pb.ChangePasswordResponse{}, f.err
}
func (f *limitedAuthClient) CreateUser(context.Context, *pb.CreateUserRequest, ...grpc.CallOption) (*pb.User, error) {
	f.creates.Add(1)
	return &pb.User{}, f.err
}

func authTestRouter(t *testing.T, cfg config.Config, client *limitedAuthClient) *gin.Engine {
	t.Helper()
	cfg.SessionCookieName = "session"
	cfg.SessionCookieMax = time.Hour
	cfg.RequestTimeout = time.Second
	r, err := NewRouter(cfg, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func postAuth(r *gin.Engine, path, ip, forwarded string, values url.Values, cookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.RemoteAddr = ip + ":1234"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", forwarded)
	if cookie {
		req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthenticationRouteLimits(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	t.Run("account limit follows normalized email across IPs", func(t *testing.T) {
		client := &limitedAuthClient{err: status.Error(codes.Unauthenticated, "private detail")}
		r := authTestRouter(t, config.Config{AuthLimits: authlimit.Config{AccountLimit: 2}}, client)
		for i, email := range []string{" Person@Example.com ", "person@example.com", "PERSON@example.com"} {
			w := postAuth(r, "/login", "192.0.2."+strconv.Itoa(i+1), "", url.Values{"email": {email}, "password": {"wrong"}}, false)
			if i < 2 && (w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "Invalid+email+or+password")) {
				t.Fatalf("unexpected login failure: %d %s", w.Code, w.Header().Get("Location"))
			}
			if i == 2 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" || strings.Contains(w.Body.String(), email)) {
				t.Fatalf("not safely throttled: %d %s", w.Code, w.Body.String())
			}
		}
		if client.logins.Load() != 2 {
			t.Fatal("blocked login reached Core")
		}
	})
	t.Run("untrusted forwarding headers cannot bypass IP limit", func(t *testing.T) {
		client := &limitedAuthClient{}
		r := authTestRouter(t, config.Config{AuthLimits: authlimit.Config{IPLimit: 2}}, client)
		for i, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
			w := postAuth(r, "/login", "192.0.2.1", "198.51.100."+strconv.Itoa(i+1), url.Values{"email": {email}, "password": {"correct"}}, false)
			if i < 2 && (w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Set-Cookie"), "session=session")) {
				t.Fatalf("valid login failed: %d %s", w.Code, w.Body.String())
			}
			if i == 2 && w.Code != http.StatusTooManyRequests {
				t.Fatal("spoofed IP bypassed limiter")
			}
		}
		if client.logins.Load() != 2 {
			t.Fatal("blocked login reached Core")
		}
	})
	t.Run("explicit trusted proxy allows distinct client IPs", func(t *testing.T) {
		client := &limitedAuthClient{}
		r := authTestRouter(t, config.Config{TrustedProxies: []string{"192.0.2.1"}, AuthLimits: authlimit.Config{IPLimit: 1}}, client)
		for _, ip := range []string{"198.51.100.1", "198.51.100.2"} {
			w := postAuth(r, "/login", "192.0.2.1", ip, url.Values{"email": {ip + "@example.com"}, "password": {"correct"}}, false)
			if w.Code != http.StatusFound {
				t.Fatalf("proxy clients share bucket: %d", w.Code)
			}
		}
	})
	t.Run("both password routes share account budget", func(t *testing.T) {
		client := &limitedAuthClient{user: &pb.User{Id: "user", Role: "user", MustChangePassword: true}}
		r := authTestRouter(t, config.Config{AuthLimits: authlimit.Config{AccountLimit: 1}}, client)
		form := url.Values{"current_password": {"old"}, "new_password": {"new"}, "confirm_new_password": {"new"}}
		w := postAuth(r, "/change-password", "192.0.2.1", "", form, true)
		if w.Code != http.StatusFound || client.changes.Load() != 1 {
			t.Fatalf("password change failed: %d", w.Code)
		}
		client.user.MustChangePassword = false
		w = postAuth(r, "/profile/password", "192.0.2.2", "", form, true)
		if w.Code != http.StatusTooManyRequests || client.changes.Load() != 1 {
			t.Fatal("password route hopping bypassed account limiter")
		}
	})
	t.Run("user creation is throttled", func(t *testing.T) {
		client := &limitedAuthClient{user: &pb.User{Id: "admin", Role: "admin"}}
		r := authTestRouter(t, config.Config{AuthLimits: authlimit.Config{AccountLimit: 1}}, client)
		form := url.Values{"email": {"new@example.com"}, "password": {"password"}, "display_name": {"Name"}}
		postAuth(r, "/admin/users", "192.0.2.1", "", form, true)
		w := postAuth(r, "/admin/users", "192.0.2.2", "", form, true)
		if w.Code != http.StatusTooManyRequests || client.creates.Load() != 1 {
			t.Fatal("user creation bypassed limiter")
		}
	})
	t.Run("oversized authentication forms never reach Core", func(t *testing.T) {
		client := &limitedAuthClient{}
		r := authTestRouter(t, config.Config{}, client)
		w := postAuth(r, "/login", "192.0.2.1", "", url.Values{"email": {"a@example.com"}, "password": {strings.Repeat("x", 20000)}}, false)
		if w.Code != http.StatusBadRequest || client.logins.Load() != 0 {
			t.Fatal("oversized form reached Core")
		}
	})
	t.Run("Core saturation returns 429 and generic errors hide details", func(t *testing.T) {
		for _, code := range []codes.Code{codes.NotFound, codes.PermissionDenied, codes.Internal, codes.ResourceExhausted} {
			client := &limitedAuthClient{err: status.Error(code, "sensitive detail")}
			r := authTestRouter(t, config.Config{}, client)
			w := postAuth(r, "/login", "192.0.2.1", "", url.Values{"email": {"a@example.com"}, "password": {"wrong"}}, false)
			if strings.Contains(w.Header().Get("Location")+w.Body.String(), "sensitive") {
				t.Fatal("Core error leaked")
			}
			if code == codes.ResourceExhausted && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "") {
				t.Fatal("Core throttling not mapped to HTTP 429")
			}
		}
	})
	t.Run("Core retry delay is preserved for every protected route", func(t *testing.T) {
		st, err := status.New(codes.ResourceExhausted, "private detail").WithDetails(&errdetails.RetryInfo{RetryDelay: durationpb.New(1500 * time.Millisecond)})
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/login", "/change-password", "/profile/password", "/admin/users"} {
			role := "user"
			if path == "/admin/users" || path == "/change-password" {
				role = "admin"
			}
			client := &limitedAuthClient{err: st.Err(), user: &pb.User{Id: "user", Role: role}}
			r := authTestRouter(t, config.Config{}, client)
			form := url.Values{"email": {"a@example.com"}, "password": {"wrong"}, "current_password": {"old"}, "new_password": {"new"}, "confirm_new_password": {"new"}}
			w := postAuth(r, path, "192.0.2.1", "", form, true)
			if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "2" || !strings.Contains(w.Body.String(), authRetryMessage) || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("wrong retry response for %s: %d %s %s", path, w.Code, w.Header().Get("Retry-After"), w.Body.String())
			}
		}
	})
}

func TestWebAuthenticationConcurrencyCap(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &limitedAuthClient{entered: make(chan struct{}), resume: make(chan struct{})}
	r := authTestRouter(t, config.Config{AuthLimits: authlimit.Config{MaxConcurrent: 1}}, client)
	done := make(chan struct{})
	go func() {
		defer close(done)
		postAuth(r, "/login", "192.0.2.1", "", url.Values{"email": {"a@example.com"}, "password": {"correct"}}, false)
	}()
	select {
	case <-client.entered:
	case <-time.After(time.Second):
		t.Fatal("login did not enter Core")
	}
	w := postAuth(r, "/login", "192.0.2.2", "", url.Values{"email": {"b@example.com"}, "password": {"correct"}}, false)
	if w.Code != http.StatusTooManyRequests || client.logins.Load() != 1 {
		t.Error("concurrent request reached Core")
	}
	close(client.resume)
	<-done
	client.entered = nil
	w = postAuth(r, "/login", "192.0.2.2", "", url.Values{"email": {"b@example.com"}, "password": {"correct"}}, false)
	if w.Code != http.StatusFound || client.logins.Load() != 2 {
		t.Fatal("completed request leaked slot")
	}
}
