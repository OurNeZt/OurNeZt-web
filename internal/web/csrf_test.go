package web

import (
	"bytes"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/authlimit"
	"github.com/OurNeZt/ournezt-web/internal/checkclient"
	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

// Existing business-flow tests must behave like a browser that first loaded a
// form. This uses the real router and issued cookie, without disabling CSRF.
func addCSRF(router http.Handler, req *http.Request) {
	page := httptest.NewRecorder()
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	get.Host = req.Host
	router.ServeHTTP(page, get)
	token := checkclient.CSRFToken(page.Body.String())
	if page.Code != http.StatusOK || token == "" {
		panic("could not load CSRF-protected login form")
	}
	scheme := "http"
	for _, cookie := range page.Result().Cookies() {
		req.AddCookie(cookie)
		if cookie.Secure {
			scheme = "https"
		}
	}
	req.Header.Set("Origin", scheme+"://"+req.Host)
	req.Header.Set("X-CSRF-Token", token)
}

func TestCSRFRejectsUnsafeSubmissionsAcrossAllPostRoutes(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := sessionClient(t, false, "user")
	router, err := NewRouter(config.Config{SessionCookieName: "session", CookieSecure: true, RequestTimeout: time.Second, AuthLimits: authlimit.Config{IPLimit: 1000}}, &core.Clients{Auth: client})
	if err != nil {
		t.Fatal(err)
	}
	seed := httptest.NewRequest(http.MethodPost, "/login", nil)
	addCSRF(router, seed)
	other := httptest.NewRequest(http.MethodPost, "/login", nil)
	addCSRF(router, other)
	validToken := seed.Header.Get("X-CSRF-Token")
	pathParams := regexp.MustCompile(`:[^/]+`)
	for _, route := range router.Routes() {
		if route.Method != http.MethodPost {
			continue
		}
		for _, scenario := range []string{"missing token", "bad token", "missing cookie", "tampered cookie", "other browser token", "query token only", "foreign origin", "sibling origin", "null origin", "wrong scheme", "wrong port"} {
			t.Run(route.Path+"/"+scenario, func(t *testing.T) {
				path := pathParams.ReplaceAllString(route.Path, "test")
				form := url.Values{csrfFieldName: {validToken}}
				switch scenario {
				case "missing token":
					form.Del(csrfFieldName)
				case "bad token":
					form.Set(csrfFieldName, "invalid")
				case "other browser token":
					form.Set(csrfFieldName, other.Header.Get("X-CSRF-Token"))
				case "query token only":
					path += "?" + form.Encode()
					form.Del(csrfFieldName)
				}
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Origin", "https://example.com")
				switch scenario {
				case "foreign origin":
					req.Header.Set("Origin", "https://attacker.invalid")
				case "sibling origin":
					req.Header.Set("Origin", "https://sibling.example.com")
				case "null origin":
					req.Header.Set("Origin", "null")
				case "wrong scheme":
					req.Header.Set("Origin", "http://example.com")
				case "wrong port":
					req.Header.Set("Origin", "https://example.com:444")
				}
				if scenario != "missing cookie" {
					for _, cookie := range seed.Cookies() {
						copy := *cookie
						if scenario == "tampered cookie" {
							copy.Value += "tampered"
						}
						req.AddCookie(&copy)
					}
				}
				req.AddCookie(&http.Cookie{Name: "session", Value: "token"})
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code != http.StatusForbidden || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "Please reload the form") || !strings.Contains(w.Body.String(), "</html>") {
					t.Fatalf("unsafe submission not rejected: %d %s", w.Code, w.Body.String())
				}
				for _, cookie := range w.Result().Cookies() {
					if cookie.Name == "session" {
						t.Fatal("rejected submission changed the session cookie")
					}
				}
			})
		}
		t.Run(route.Path+"/valid token reaches authentication or form validation", func(t *testing.T) {
			form := url.Values{csrfFieldName: {validToken}}
			req := httptest.NewRequest(http.MethodPost, pathParams.ReplaceAllString(route.Path, "test"), strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "https://example.com")
			for _, cookie := range seed.Cookies() {
				req.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			// With no session/credentials, valid submissions follow the usual
			// login redirect rather than being rejected by CSRF protection.
			if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?") {
				t.Fatalf("valid CSRF token did not pass: %d %s", w.Code, w.Header().Get("Location"))
			}
		})
	}
	if client.validations != 0 || client.revocations != 0 || client.changes != 0 {
		t.Fatal("rejected CSRF requests reached Core")
	}
}

func TestCSRFFormsRemainValidAcrossTabs(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	client := &limitedAuthClient{}
	router := authTestRouter(t, config.Config{}, client)
	first := httptest.NewRequest(http.MethodPost, "/login", nil)
	addCSRF(router, first)
	page := httptest.NewRecorder()
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	for _, cookie := range first.Cookies() {
		get.AddCookie(cookie)
	}
	router.ServeHTTP(page, get)
	secondToken := checkclient.CSRFToken(page.Body.String())
	if secondToken == "" || secondToken == first.Header.Get("X-CSRF-Token") || len(page.Result().Cookies()) != 0 {
		t.Fatal("second tab did not receive a fresh mask for the existing CSRF cookie")
	}
	for _, token := range []string{first.Header.Get("X-CSRF-Token"), secondToken} {
		form := url.Values{"email": {"person@example.com"}, "password": {"password"}, csrfFieldName: {token}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range first.Cookies() {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusFound {
			t.Fatalf("an existing tab's token stopped working: %d", w.Code)
		}
	}
	if client.logins.Load() != 2 {
		t.Fatal("both tabs did not reach login")
	}
}

func TestCSRFValidFormsAndOriginFallback(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, secure := range []bool{false, true} {
		client := &limitedAuthClient{}
		router := authTestRouter(t, config.Config{CookieSecure: secure}, client)
		seed := httptest.NewRequest(http.MethodPost, "/login", nil)
		addCSRF(router, seed)
		token := seed.Header.Get("X-CSRF-Token")
		origin := seed.Header.Get("Origin")
		for _, scenario := range []string{"form", "header", "same-origin referer", "foreign referer", "no origin headers", "foreign origin with valid referer"} {
			t.Run(origin+"/"+scenario, func(t *testing.T) {
				form := url.Values{"email": {"person@example.com"}, "password": {"password"}, csrfFieldName: {token}}
				if scenario == "header" {
					form.Del(csrfFieldName)
				}
				req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				for _, cookie := range seed.Cookies() {
					req.AddCookie(cookie)
				}
				switch scenario {
				case "form", "header":
					req.Header.Set("Origin", origin)
				case "same-origin referer":
					req.Header.Set("Referer", origin+"/login")
				case "foreign referer":
					req.Header.Set("Referer", "https://attacker.invalid/login")
				case "foreign origin with valid referer":
					req.Header.Set("Origin", "https://attacker.invalid")
					req.Header.Set("Referer", origin+"/login")
				}
				if scenario == "header" {
					req.Header.Set("X-CSRF-Token", token)
				}
				before := client.logins.Load()
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				rejected := scenario == "foreign origin with valid referer" || scenario == "foreign referer" || secure && scenario == "no origin headers"
				if rejected {
					if w.Code != http.StatusForbidden || client.logins.Load() != before {
						t.Fatalf("invalid origin reached login: %d", w.Code)
					}
				} else if w.Code != http.StatusFound || client.logins.Load() != before+1 {
					t.Fatalf("valid CSRF submission did not reach login: %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestCSRFCookieAndProductionConfiguration(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	key := strings.Repeat("ab", 32)
	for _, value := range []string{"", "secret", strings.Repeat("a", 62), strings.Repeat("z", 64)} {
		if _, err := NewRouter(config.Config{AppEnv: "production", CSRFAuthKey: value}, &core.Clients{}); err == nil {
			t.Fatal("production accepted a missing or invalid CSRF secret")
		}
	}
	cfg := config.Config{AppEnv: "production", CSRFAuthKey: key}
	router, err := NewRouter(cfg, &core.Clients{})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookies := w.Result().Cookies()
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || len(cookies) != 1 {
		t.Fatal("login did not issue a noncacheable CSRF-protected form")
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-ournezt_csrf" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe CSRF cookie: %v", cookie)
	}
	// A second replica using the same key must accept the first replica's form.
	second, err := NewRouter(cfg, &core.Clients{})
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{csrfFieldName: {checkclient.CSRFToken(w.Body.String())}}
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://example.com")
	result := httptest.NewRecorder()
	second.ServeHTTP(result, req)
	if result.Code != http.StatusFound {
		t.Fatalf("shared-key replica rejected the form: %d", result.Code)
	}
}

func TestCSRFProtectsFutureUnsafeMethods(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	router, err := NewRouter(config.Config{}, &core.Clients{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		router.Handle(method, "/future", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		missing := httptest.NewRecorder()
		router.ServeHTTP(missing, httptest.NewRequest(method, "/future", nil))
		valid := httptest.NewRequest(method, "/future", nil)
		addCSRF(router, valid)
		accepted := httptest.NewRecorder()
		router.ServeHTTP(accepted, valid)
		if missing.Code != http.StatusForbidden || accepted.Code != http.StatusNoContent {
			t.Fatalf("%s CSRF enforcement failed: rejected=%d accepted=%d", method, missing.Code, accepted.Code)
		}
	}
}

// Parse rendered HTML so broken nesting or lost template scope cannot silently
// produce a form whose token is outside it. GET forms must not leak tokens.
func assertCSRFForms(t *testing.T, page, token string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	attr := func(n *html.Node, key string) string {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			count := 0
			var inputs func(*html.Node)
			inputs = func(input *html.Node) {
				if input.Type == html.ElementNode && input.Data == "input" && attr(input, "name") == csrfFieldName {
					count++
					if attr(input, "type") != "hidden" || attr(input, "value") != token {
						t.Errorf("form %s has an invalid CSRF field", attr(n, "action"))
					}
				}
				for child := input.FirstChild; child != nil; child = child.NextSibling {
					inputs(child)
				}
			}
			inputs(n)
			want := 0
			if strings.EqualFold(attr(n, "method"), "post") {
				want = 1
			}
			if count != want {
				t.Errorf("form %s has %d tokens, want %d", attr(n, "action"), count, want)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
}

func TestCSRFFieldsInAllFormTemplates(t *testing.T) {
	tpl, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	token := base64.StdEncoding.EncodeToString(make([]byte, 64))
	field := template.HTML(`<input type="hidden" name="csrf_token" value="` + token + `">`)
	person := &pb.PersonProfile{Id: "person"}
	home := &pb.HousingOption{Id: "home", FamilyId: "family"}
	criterion := &pb.HousingCriterion{Id: "criterion", Name: "Location"}
	detail := housingDetailData{Housing: home, Affordability: &pb.HousingAffordability{}, Evaluation: &pb.HousingEvaluation{Criteria: []*pb.HousingCriterion{criterion}}, EvaluationRows: []housingEvaluationRow{{Criterion: criterion, Answer: &pb.HousingAnswer{State: "pending"}}}}
	for _, tc := range []struct {
		name string
		data any
	}{
		{"login", loginPayload{}},
		{"change_password", changePasswordPayload{}},
		{"profile", profilePayload{User: &CurrentUser{ID: "user"}}},
		{"admin_users", adminUsersData{Users: []*pb.User{{Id: "other"}}}},
		{"families", familiesData{}},
		{"family_detail", familyDetailData{Family: &pb.Family{Id: "family"}}},
		{"people", peopleData{FamilyID: "family", Rows: []peopleRow{{Person: person}}}},
		{"person_form", personFormData{Person: person}},
		{"housing", housingData{FamilyID: "family", GroupedSections: []housingGroupSection{{GroupID: "group", HousingOptions: []*pb.HousingOption{home}}}}},
		{"housing_form", housingFormData{Housing: home}},
		{"housing_checklist", housingChecklistData{FamilyID: "family", Draft: &pb.HousingCriterion{}, Criteria: []*pb.HousingCriterion{criterion}}},
		{"housing_detail", detail},
		{"housing_notes_error", detail},
		{"core_unavailable", coreFailureData{Logout: true, Notes: &detail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var page bytes.Buffer
			if err := tpl.ExecuteTemplate(&page, tc.name, ViewData{CSRFField: field, User: &CurrentUser{ID: "user", Role: "user"}, Data: tc.data}); err != nil {
				t.Fatal(err)
			}
			assertCSRFForms(t, page.String(), token)
		})
	}
}
