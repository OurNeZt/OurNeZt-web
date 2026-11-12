package web

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/authlimit"
	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
	"google.golang.org/grpc/codes"
)

func assertPageSecurity(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	policy := w.Header().Get("Content-Security-Policy")
	match := regexp.MustCompile(`script-src 'nonce-([^']+)'`).FindStringSubmatch(policy)
	if len(match) != 2 {
		t.Fatalf("missing nonce-only script policy: %s", policy)
	}
	nonce := match[1]
	decoded, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("invalid CSP nonce: %q", nonce)
	}
	for _, required := range []string{"default-src 'none'", "script-src-attr 'none'", "style-src 'self'", "style-src-attr 'none'", "form-action 'self'", "connect-src 'self'", "base-uri 'none'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(policy, required) {
			t.Errorf("policy missing %s", required)
		}
	}
	if strings.Contains(policy, "unsafe-") || strings.Contains(policy, "https:") || strings.Contains(policy, "*") {
		t.Fatalf("overly permissive policy: %s", policy)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing companion security headers")
	}
	if strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("nonce-bearing HTML can be cached")
		}
		tokens := html.NewTokenizer(strings.NewReader(w.Body.String()))
		for tokens.Next() != html.ErrorToken {
			token := tokens.Token()
			if token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken {
				continue
			}
			attrs := map[string]string{}
			for _, attr := range token.Attr {
				attrs[attr.Key] = attr.Val
				if strings.HasPrefix(attr.Key, "on") || attr.Key == "style" {
					t.Errorf("CSP-incompatible attribute: %s", attr.Key)
				}
			}
			if token.Data == "style" {
				t.Error("inline styles are blocked by the policy")
			}
			if token.Data == "script" && attrs["nonce"] != nonce {
				t.Errorf("script nonce does not match response policy: %v", attrs)
			}
			var asset string
			if token.Data == "script" {
				asset = attrs["src"]
			} else if token.Data == "link" && attrs["rel"] == "stylesheet" {
				asset = attrs["href"]
			}
			if asset != "" {
				if !strings.HasPrefix(asset, "/static/") {
					t.Errorf("nonlocal frontend asset: %q", asset)
				} else if _, err := os.Stat(strings.SplitN(strings.TrimPrefix(asset, "/"), "?", 2)[0]); err != nil {
					t.Errorf("missing frontend asset %q: %v", asset, err)
				}
			}
		}
	}
	return nonce
}

// Fixtures exercise each script-bearing page and the nested notes error partial.
func securityTemplateRouter(t *testing.T) *gin.Engine {
	t.Helper()
	tpl, err := parseTemplates("templates")
	if err != nil {
		t.Fatal(err)
	}
	user := &CurrentUser{ID: "viewer", Role: "user"}
	app := &App{}
	router := gin.New()
	router.Use(securityHeaders())
	router.SetHTMLTemplate(tpl)
	router.Static("/static", "static")
	for name, data := range map[string]any{
		"dashboard":           dashboardData{Dashboard: &pb.HouseholdDashboard{}, Income: &pb.HouseholdIncomeSummary{}, IncomeTrendLabels: []string{"Nov 2026"}, IncomeTrendGrossCents: []int64{100000}, HousingCharts: []dashboardHousingChartRow{{Name: "Home"}}, TimelineSeries: []dashboardTimelineSeries{{HousingID: "home", HousingName: "Home", YearNow: 2026}}},
		"admin_users":         adminUsersData{},
		"person_form":         personFormData{Person: &pb.PersonProfile{}},
		"housing_form":        housingFormData{Housing: &pb.HousingOption{}},
		"housing":             housingData{},
		"housing_checklist":   housingChecklistData{Draft: &pb.HousingCriterion{}},
		"housing_notes_error": housingDetailData{Housing: &pb.HousingOption{Id: "home"}, NotesMaxLength: 10000},
		"profile":             profilePayload{User: user},
		"change_password":     changePasswordPayload{},
	} {
		router.GET("/"+name, func(c *gin.Context) {
			c.Set(userContextKey, user)
			app.render(c, name, "Security fixture", data)
		})
	}
	router.GET("/core-error", func(c *gin.Context) {
		c.Set("housing_notes_draft", "Preserved draft")
		c.Params = gin.Params{{Key: "id", Value: "home"}}
		app.renderCoreFailure(c, codes.Unavailable)
	})
	return router
}

func TestSecurityHeadersAuthorizePageScripts(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	router := securityTemplateRouter(t)
	seen := map[string]bool{}
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/static/") {
			continue
		}
		t.Run(route.Path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route.Path, nil))
			if w.Code != http.StatusOK && (route.Path != "/core-error" || w.Code != http.StatusServiceUnavailable) {
				t.Fatalf("unexpected fixture status: %d", w.Code)
			}
			nonce := assertPageSecurity(t, w)
			if seen[nonce] {
				t.Fatal("CSP nonce reused across responses")
			}
			seen[nonce] = true
		})
	}
}

func TestSecurityHeadersCoverRouterAndEarlyRejections(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	router, err := NewRouter(config.Config{SessionCookieName: "session", RequestTimeout: time.Second, AuthLimits: authlimit.Config{IPLimit: 1}}, &core.Clients{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/login", 200},
		{http.MethodGet, "/login", 200},
		{http.MethodGet, "/dashboard", 302},
		{http.MethodGet, "/missing", 404},
		{http.MethodPost, "/login", 403},
		{http.MethodPost, "/login", 429},
		{http.MethodGet, "/static/vendor/ui.css", 200},
		{http.MethodGet, "/static/vendor/chart.umd.js", 200},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
		nonce := assertPageSecurity(t, w)
		if seen[nonce] {
			t.Fatal("CSP nonce reused")
		}
		seen[nonce] = true
	}
}

func TestVendoredFrontendChecksums(t *testing.T) {
	root := filepath.Join("..", "..", "static", "vendor")
	sums, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid checksum entry: %s", line)
		}
		content, err := os.ReadFile(filepath.Join(root, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(content)) != fields[0] {
			t.Errorf("vendored asset changed without regeneration: %s", fields[1])
		}
	}
}
