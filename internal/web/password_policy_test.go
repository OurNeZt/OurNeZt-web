package web

import (
	"context"
	"net/http"
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
)

type passwordPolicyAuthClient struct {
	*revocationAuthClient
	created *pb.CreateUserRequest
	changed *pb.ChangePasswordRequest
	login   *pb.LoginRequest
}

func (f *passwordPolicyAuthClient) CreateUser(_ context.Context, req *pb.CreateUserRequest, _ ...grpc.CallOption) (*pb.User, error) {
	f.created = req
	return &pb.User{Id: "new-user", Email: req.Email}, nil
}

func (f *passwordPolicyAuthClient) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest, options ...grpc.CallOption) (*pb.ChangePasswordResponse, error) {
	f.changed = req
	return f.revocationAuthClient.ChangePassword(ctx, req, options...)
}

func (f *passwordPolicyAuthClient) Login(_ context.Context, req *pb.LoginRequest, _ ...grpc.CallOption) (*pb.LoginResponse, error) {
	f.login = req
	return &pb.LoginResponse{SessionToken: "token", User: f.sessions["token"]}, nil
}

func passwordPolicyRouter(t *testing.T, client *passwordPolicyAuthClient) *gin.Engine {
	t.Helper()
	router, err := NewRouter(config.Config{SessionCookieName: "session", CookieSecure: true, RequestTimeout: time.Second}, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestPasswordPolicyWhitespaceRejectedByEveryForm(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, route := range []struct {
		path, role string
		forced     bool
	}{{"/admin/users", "admin", false}, {"/change-password", "user", true}, {"/profile/password", "user", false}} {
		for _, tc := range []struct{ name, password string }{
			{"leading", " quiet-otter-bay"},
			{"trailing", "quiet-otter-bay "},
			{"internal", "quiet otter bay"},
			{"tab", "quiet\totter-bay"},
			{"newline", "quiet-otter\nbay"},
			{"nonbreaking space", "quiet\u00a0otter-bay"},
			{"unicode space", "quiet\u2003otter-bay"},
			{"whitespace only", strings.Repeat(" ", 20)},
		} {
			t.Run(route.path+"/"+tc.name, func(t *testing.T) {
				client := &passwordPolicyAuthClient{revocationAuthClient: sessionClient(t, route.forced, route.role)}
				router := passwordPolicyRouter(t, client)
				form := url.Values{"email": {"review@example.com"}, "display_name": {"Review"}, "password": {tc.password}, "current_password": {"old-password"}, "new_password": {tc.password}, "confirm_new_password": {tc.password}}
				w := postSessionForm(router, route.path, "token", form)
				location, err := url.Parse(w.Header().Get("Location"))
				if err != nil || w.Code != http.StatusFound || location.Query().Get("error") != passwordWhitespaceMessage {
					t.Fatalf("missing clear validation error: status=%d location=%s err=%v", w.Code, w.Header().Get("Location"), err)
				}
				if client.created != nil || client.changed != nil || client.changes != 0 || client.revocations != 0 || len(client.sessions) != 3 || client.sessions["token"].MustChangePassword != route.forced || w.Header().Get("Set-Cookie") != "" {
					t.Fatal("rejected password reached a Core mutation or altered sessions/first-login state")
				}
			})
		}
	}
}

func TestPasswordPolicyCurrentPasswordPreservedAndValidChangeRevokesSessions(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, route := range []struct {
		path   string
		forced bool
	}{{"/change-password", true}, {"/profile/password", false}} {
		t.Run(route.path, func(t *testing.T) {
			client := &passwordPolicyAuthClient{revocationAuthClient: sessionClient(t, route.forced, "user")}
			const current = "  existing password  "
			const replacement = "a-peaceful-garden-near-home"
			form := url.Values{"current_password": {current}, "new_password": {replacement}, "confirm_new_password": {replacement}}
			w := postSessionForm(passwordPolicyRouter(t, client), route.path, "token", form)
			if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?flash=") || client.changed == nil || client.changed.CurrentPassword != current || client.changed.NewPassword != replacement || client.changes != 1 {
				t.Fatalf("password values changed or valid submission rejected: %d %s", w.Code, w.Header().Get("Location"))
			}
			if client.sessions["token"] != nil || client.sessions["copied-token"] != nil || client.sessions["other-user-token"] == nil {
				t.Fatal("valid password change did not revoke only the user's sessions")
			}
			requireClearedSessionCookie(t, w)
		})
	}
}

func TestPasswordPolicyConfirmationIsNotTrimmed(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &passwordPolicyAuthClient{revocationAuthClient: sessionClient(t, false, "user")}
	form := url.Values{"current_password": {"old"}, "new_password": {"quiet-otter-bay"}, "confirm_new_password": {" quiet-otter-bay "}}
	w := postSessionForm(passwordPolicyRouter(t, client), "/profile/password", "token", form)
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Query().Get("error") != "new password confirmation does not match" || client.changed != nil || len(client.sessions) != 3 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("confirmation was silently trimmed or a mismatched password was submitted")
	}
}

func TestPasswordPolicyLegacyLoginAndNewUserCreation(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &passwordPolicyAuthClient{revocationAuthClient: sessionClient(t, false, "admin")}
	router := passwordPolicyRouter(t, client)
	const legacy = "  existing password  "
	w := postSessionForm(router, "/login", "", url.Values{"email": {"review@example.com"}, "password": {legacy}})
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/dashboard?") || client.login == nil || client.login.Password != legacy {
		t.Fatal("legacy whitespace password was modified or blocked at login")
	}
	w = postSessionForm(router, "/admin/users", "token", url.Values{"email": {"new@example.com"}, "display_name": {"New"}, "password": {"quiet-otter-bay"}})
	if w.Code != http.StatusSeeOther || client.created == nil || client.created.Password != "quiet-otter-bay" {
		t.Fatal("compliant password was blocked or altered when creating a user")
	}
}
