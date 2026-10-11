package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type revocationAuthClient struct {
	pb.AuthServiceClient
	sessions                          map[string]*pb.User
	validations, revocations, changes int
	revokeError, changeError          error
	stall                             bool
	t                                 *testing.T
	beforeRevoke                      func()
}

func sessionClient(t *testing.T, mustChange bool, role string) *revocationAuthClient {
	t.Helper()
	user := &pb.User{Id: "user", Role: role, MustChangePassword: mustChange}
	return &revocationAuthClient{t: t, sessions: map[string]*pb.User{"token": user, "copied-token": user, "other-user-token": {Id: "other", Role: "user"}}}
}

func (f *revocationAuthClient) ValidateSession(_ context.Context, req *pb.ValidateSessionRequest, _ ...grpc.CallOption) (*pb.ValidateSessionResponse, error) {
	f.validations++
	user, ok := f.sessions[req.GetSessionToken()]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid session")
	}
	return &pb.ValidateSessionResponse{User: user}, nil
}

func (f *revocationAuthClient) token(ctx context.Context) string {
	f.t.Helper()
	if _, ok := ctx.Deadline(); !ok {
		f.t.Error("session operation lacks a deadline")
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	tokens := md.Get("x-session-token")
	if len(tokens) != 1 {
		f.t.Errorf("missing or duplicated session metadata: %v", tokens)
		return ""
	}
	return tokens[0]
}

func (f *revocationAuthClient) RevokeCurrentSession(ctx context.Context, _ *pb.RevokeCurrentSessionRequest, _ ...grpc.CallOption) (*pb.RevokeCurrentSessionResponse, error) {
	f.revocations++
	token := f.token(ctx)
	if f.beforeRevoke != nil {
		f.beforeRevoke()
	}
	if f.stall {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if f.revokeError != nil {
		return nil, f.revokeError
	}
	delete(f.sessions, token)
	return &pb.RevokeCurrentSessionResponse{}, nil
}

func (f *revocationAuthClient) ChangePassword(ctx context.Context, _ *pb.ChangePasswordRequest, _ ...grpc.CallOption) (*pb.ChangePasswordResponse, error) {
	f.changes++
	token := f.token(ctx)
	if f.changeError != nil {
		return nil, f.changeError
	}
	user := f.sessions[token]
	if user == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid session")
	}
	for token, sessionUser := range f.sessions {
		if sessionUser.Id == user.Id {
			delete(f.sessions, token)
		}
	}
	return &pb.ChangePasswordResponse{}, nil
}

func revocationRouter(t *testing.T, client *revocationAuthClient, timeout time.Duration) *gin.Engine {
	t.Helper()
	router, err := NewRouter(config.Config{SessionCookieName: "session", CookieSecure: true, RequestTimeout: timeout}, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func postSessionForm(router *gin.Engine, path, token string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: token})
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func requireClearedSessionCookie(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "session" || cookies[0].Value != "" || cookies[0].MaxAge != -1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" {
		t.Fatalf("session cookie was not safely cleared: %v", cookies)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("session response is cacheable")
	}
}

func TestSessionLogoutRevokesBeforeClearingCookie(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := sessionClient(t, false, "user")
	router := revocationRouter(t, client, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	w := httptest.NewRecorder()
	client.beforeRevoke = func() {
		if w.Header().Get("Set-Cookie") != "" {
			t.Error("cookie cleared before Core revocation")
		}
	}
	router.ServeHTTP(w, req)
	if w.Code != http.StatusFound || client.revocations != 1 || client.validations != 0 {
		t.Fatalf("logout did not directly revoke the session: %d", w.Code)
	}
	if client.sessions["token"] != nil || client.sessions["copied-token"] == nil || client.sessions["other-user-token"] == nil {
		t.Fatal("logout revoked the wrong sessions")
	}
	requireClearedSessionCookie(t, w)
	// A copied browser cookie with the revoked token no longer authenticates.
	get := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	get.AddCookie(&http.Cookie{Name: "session", Value: "token"})
	reuse := httptest.NewRecorder()
	router.ServeHTTP(reuse, get)
	if reuse.Code != http.StatusFound || !strings.HasPrefix(reuse.Header().Get("Location"), "/login?") {
		t.Fatal("logged-out token still authenticated")
	}
	client.beforeRevoke = nil
	// Retrying logout with a copied, expired, or unknown token remains safe.
	again := postSessionForm(router, "/logout", "token", nil)
	if again.Code != http.StatusFound {
		t.Fatalf("repeat logout failed: %d", again.Code)
	}
	requireClearedSessionCookie(t, again)
}

func TestSessionLogoutFailureKeepsCookieAndOffersRetry(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.Unimplemented, codes.DeadlineExceeded} {
		t.Run(code.String(), func(t *testing.T) {
			client := sessionClient(t, false, "user")
			client.revokeError = status.Error(code, "private Core details")
			w := postSessionForm(revocationRouter(t, client, time.Second), "/logout", "token", nil)
			expected := http.StatusServiceUnavailable
			if code == codes.DeadlineExceeded {
				expected = http.StatusGatewayTimeout
			}
			if w.Code != expected || w.Header().Get("Set-Cookie") != "" || client.sessions["token"] == nil || !strings.Contains(w.Body.String(), "Retry logout") || strings.Contains(w.Body.String(), "private Core") || !strings.Contains(w.Body.String(), "</html>") {
				t.Fatalf("unsafe failed logout response: %d %s", w.Code, w.Body.String())
			}
		})
	}
	client := sessionClient(t, false, "user")
	client.stall = true
	started := time.Now()
	w := postSessionForm(revocationRouter(t, client, 20*time.Millisecond), "/logout", "token", nil)
	if w.Code != http.StatusGatewayTimeout || w.Header().Get("Set-Cookie") != "" || time.Since(started) > time.Second {
		t.Fatal("stalled revocation was not bounded")
	}
}

func TestSessionLogoutWithoutCookieDoesNotCallCore(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := sessionClient(t, false, "user")
	w := postSessionForm(revocationRouter(t, client, time.Second), "/logout", "", nil)
	if w.Code != http.StatusFound || client.revocations != 0 || client.validations != 0 {
		t.Fatal("cookie-free logout called Core")
	}
	requireClearedSessionCookie(t, w)
}

func passwordForm() url.Values {
	return url.Values{"current_password": {"old-password"}, "new_password": {"new-password"}, "confirm_new_password": {"new-password"}}
}

func TestSessionPasswordChangeRequiresFreshLogin(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, tc := range []struct {
		path, role string
		mustChange bool
	}{
		{"/profile/password", "user", false}, {"/change-password", "user", true}, {"/change-password", "admin", true}, {"/change-password", "admin", false},
	} {
		t.Run(fmt.Sprintf("%s/%s/first-login=%t", tc.path, tc.role, tc.mustChange), func(t *testing.T) {
			client := sessionClient(t, tc.mustChange, tc.role)
			w := postSessionForm(revocationRouter(t, client, time.Second), tc.path, "token", passwordForm())
			location, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusFound || location.Path != "/login" || !strings.Contains(location.Query().Get("flash"), "log in again") || client.changes != 1 {
				t.Fatalf("password change did not require reauthentication: %d %s", w.Code, location)
			}
			if client.sessions["token"] != nil || client.sessions["copied-token"] != nil || client.sessions["other-user-token"] == nil {
				t.Fatal("password change did not revoke all of the user's sessions")
			}
			requireClearedSessionCookie(t, w)
		})
	}
}

func TestSessionFailedPasswordChangePreservesCookie(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, path := range []string{"/profile/password", "/change-password"} {
		for _, code := range []codes.Code{codes.Unauthenticated, codes.Unavailable, codes.DeadlineExceeded} {
			client := sessionClient(t, path == "/change-password", "user")
			client.changeError = status.Error(code, "private backend details")
			w := postSessionForm(revocationRouter(t, client, time.Second), path, "token", passwordForm())
			if w.Header().Get("Set-Cookie") != "" || client.sessions["token"] == nil || client.changes != 1 || strings.Contains(w.Header().Get("Location"), "private") {
				t.Fatal("failed password change cleared the session or leaked details")
			}
		}
	}
}

func TestSessionLogoutAlreadyInvalidTokenClearsCookie(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := sessionClient(t, false, "user")
	delete(client.sessions, "token")
	client.revokeError = status.Error(codes.Unauthenticated, "session already invalid")
	w := postSessionForm(revocationRouter(t, client, time.Second), "/logout", "token", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("Core-confirmed invalid session prevented logout: %d", w.Code)
	}
	requireClearedSessionCookie(t, w)
}
