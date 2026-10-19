package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OurNeZt/ournezt-web/internal/checkclient"
	"github.com/OurNeZt/ournezt-web/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoginFailuresHaveIdenticalResponses(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	var failures []error
	for _, code := range []codes.Code{codes.Unauthenticated, codes.NotFound, codes.PermissionDenied, codes.FailedPrecondition, codes.InvalidArgument, codes.Internal, codes.Unknown, codes.Unavailable, codes.DeadlineExceeded, codes.Canceled} {
		failures = append(failures, status.Error(code, "private account detail"))
	}
	failures = append(failures, errors.New("private lookup detail"))
	var baseline string
	for _, failure := range failures {
		t.Run(failure.Error(), func(t *testing.T) {
			client := &limitedAuthClient{err: failure}
			r := authTestRouter(t, config.Config{}, client)
			w := postAuth(r, "/login", "192.0.2.1", "", url.Values{"email": {"person@example.com"}, "password": {"wrong"}}, false)
			if w.Code != http.StatusFound || w.Header().Get("Location") != "/login?error=Invalid+email+or+password." || len(w.Result().Cookies()) != 0 {
				t.Fatalf("unsafe login failure: %d %v %s", w.Code, w.Header(), w.Body.String())
			}
			page := httptest.NewRecorder()
			r.ServeHTTP(page, httptest.NewRequest(http.MethodGet, w.Header().Get("Location"), nil))
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Invalid email or password.") || strings.Contains(page.Body.String(), "private") {
				t.Fatalf("login page exposed an unsafe message: %d %s", page.Code, page.Body.String())
			}
			body := checkclient.NormalizeCSRF(page.Body.String())
			if baseline == "" {
				baseline = body
			} else if body != baseline {
				t.Fatal("login pages differ by backend failure state")
			}
		})
	}
}

func TestLoginEmptyCredentialsUseGenericFailure(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, form := range []url.Values{{}, {"email": {"person@example.com"}}, {"password": {"wrong"}}} {
		client := &limitedAuthClient{}
		r := authTestRouter(t, config.Config{}, client)
		w := postAuth(r, "/login", "192.0.2.1", "", form, false)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/login?error=Invalid+email+or+password." || client.logins.Load() != 0 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("empty credentials produced a different failure: %d %v", w.Code, w.Header())
		}
	}
}
