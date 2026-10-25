package web

import (
	"bytes"
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
	"golang.org/x/net/html"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestPasswordPolicyCoreValidationReasonsAreActionable(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, route := range []struct {
		path   string
		forced bool
	}{{"/change-password", true}, {"/profile/password", false}} {
		for _, tc := range []struct{ reason, message string }{
			{"password must not contain whitespace", passwordWhitespaceMessage},
			{"password must not be blank or whitespace-only", passwordWhitespaceMessage},
			{"password must contain at least 15 characters", "new password must contain at least 15 characters"},
			{"password must not exceed 1024 bytes", "new password must not exceed 1,024 UTF-8 bytes"},
			{"new_password must not exceed 1024 bytes", "new password must not exceed 1,024 UTF-8 bytes"},
			{"current_password must not exceed 1024 bytes", "current password must not exceed 1,024 UTF-8 bytes"},
			{"password is too common; choose a less predictable password", "new password is too common; choose a less predictable password"},
			{"password must be valid UTF-8", "new password must contain valid UTF-8 text"},
			{"private account detail and submitted-secret", "password validation failed; check the password requirements"},
		} {
			t.Run(route.path+"/"+tc.reason, func(t *testing.T) {
				client := &passwordPolicyAuthClient{revocationAuthClient: sessionClient(t, route.forced, "user")}
				client.changeError = status.Error(codes.InvalidArgument, "invalid argument: "+tc.reason)
				form := url.Values{"current_password": {"old-password"}, "new_password": {"quiet-otter-bay"}, "confirm_new_password": {"quiet-otter-bay"}}
				w := postSessionForm(passwordPolicyRouter(t, client), route.path, "token", form)
				location, err := url.Parse(w.Header().Get("Location"))
				if err != nil || w.Code != http.StatusFound || location.Query().Get("error") != tc.message || strings.Contains(location.String(), "submitted-secret") {
					t.Fatalf("incorrect or unsafe validation message: status=%d location=%s", w.Code, w.Header().Get("Location"))
				}
				if client.changes != 1 || len(client.sessions) != 3 || client.sessions["token"].MustChangePassword != route.forced || w.Header().Get("Set-Cookie") != "" {
					t.Fatal("failed Core validation altered credentials, sessions, or first-login state")
				}
			})
		}
	}
}

func passwordPolicyTemplateFixtures(t *testing.T) map[string]string {
	t.Helper()
	templates, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := make(map[string]string)
	reviewer := &CurrentUser{ID: "reviewer", Role: "admin"}
	for _, tc := range []struct {
		name string
		data any
	}{
		{"change_password", changePasswordPayload{IsForced: true}},
		{"profile", profilePayload{User: reviewer}},
		{"admin_users", adminUsersData{}},
		{"login", loginPayload{}},
	} {
		var output bytes.Buffer
		if err := templates.ExecuteTemplate(&output, tc.name, ViewData{Title: "Password review", Data: tc.data, User: reviewer}); err != nil {
			t.Fatal(err)
		}
		fixtures[tc.name] = output.String()
	}
	return fixtures
}

func TestPasswordPolicyBrowserHooksOnlyAffectNewPasswords(t *testing.T) {
	for name, fixture := range passwordPolicyTemplateFixtures(t) {
		t.Run(name, func(t *testing.T) {
			fields := make(map[string]bool)
			tokens := html.NewTokenizer(strings.NewReader(fixture))
			for tokens.Next() != html.ErrorToken {
				token := tokens.Token()
				if token.Data != "input" {
					continue
				}
				var field string
				guarded := false
				for _, attr := range token.Attr {
					if attr.Key == "name" {
						field = attr.Val
					}
					if attr.Key == "data-password-no-whitespace" {
						guarded = true
					}
				}
				if guarded {
					fields[field] = true
				}
			}
			if name == "login" {
				if len(fields) != 0 {
					t.Fatal("browser guard would block legacy login passwords")
				}
				return
			}
			if !strings.Contains(fixture, `src="/static/js/password-policy.js"`) {
				t.Fatal("password form is missing immediate feedback script")
			}
			if name == "admin_users" {
				if len(fields) != 1 || !fields["password"] {
					t.Fatalf("unexpected temporary-password guards: %v", fields)
				}
			} else if len(fields) != 2 || !fields["new_password"] || !fields["confirm_new_password"] {
				t.Fatalf("browser guard is missing or affects a current password: %v", fields)
			}
		})
	}
}
