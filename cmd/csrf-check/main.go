// csrf-check probes a running local/staging Web instance without signing in.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/checkclient"
	"golang.org/x/net/publicsuffix"
)

type settings struct {
	baseURL   string
	timeout   time.Duration
	transport http.RoundTripper // Allows tests to trust their local TLS server.
}

type routeGroup struct {
	name   string
	routes []string
}

var postGroups = []routeGroup{
	{"login and logout", []string{"/login", "/logout"}},
	{"password and account", []string{"/change-password", "/profile/account", "/profile/password", "/profile/person", "/profile/person/:id"}},
	{"admin", []string{"/admin/users", "/admin/users/:id/disable"}},
	{"family", []string{"/families", "/families/join", "/families/:id/invite-code/regenerate"}},
	{"person", []string{"/people", "/people/:id", "/people/:id/delete"}},
	{"housing", []string{
		"/housing", "/housing/:id", "/housing/:id/notes", "/housing/:id/group", "/housing/:id/visibility", "/housing/:id/delete",
		"/housing/groups", "/housing/groups/:id", "/housing/groups/:id/delete", "/housing/groups/:id/visibility",
		"/housing/checklist", "/housing/checklist/:criterion_id", "/housing/checklist/:criterion_id/delete", "/housing/:id/evaluation/:criterion_id",
	}},
}

func main() {
	var cfg settings
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8080", "Web origin for a local/staging deployment")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "Timeout for each HTTP request")
	flag.Parse()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg settings, out io.Writer) error {
	u, err := url.Parse(cfg.baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("-url must be an HTTP(S) origin without credentials, a path, query, or fragment")
	}
	if cfg.timeout <= 0 {
		return fmt.Errorf("-timeout must be positive")
	}
	base := strings.TrimRight(u.String(), "/")
	newClient := func() (*http.Client, error) {
		jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		if err != nil {
			return nil, err
		}
		return &http.Client{Timeout: cfg.timeout, Jar: jar, Transport: cfg.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	load := func(client *http.Client) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/login", nil)
		if err != nil {
			return "", err
		}
		resp, page, err := request(client, req)
		if err != nil {
			return "", fmt.Errorf("cannot load the login form: %w", err)
		}
		token := checkclient.CSRFToken(page)
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") || token == "" || len(token) > 256 {
			return "", fmt.Errorf("login page must return HTTP 200 and contain a hidden CSRF token")
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			return "", fmt.Errorf("login page containing a token must use Cache-Control: no-store")
		}
		return token, nil
	}
	first, err := load(client)
	if err != nil {
		return err
	}
	second, err := load(client)
	if err != nil {
		return err
	}
	if first == second {
		return fmt.Errorf("separate page loads must receive different masked CSRF tokens")
	}
	if _, err := fmt.Fprintln(out, "PASS: login forms include fresh CSRF tokens and cannot be cached."); err != nil {
		return err
	}
	post := func(client *http.Client, path, token, origin, referer string, rejected bool) error {
		form := url.Values{}
		if token != "" {
			form.Set("csrf_token", token)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, _, err := request(client, req)
		if err != nil {
			return fmt.Errorf("%s request failed: %w", path, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("%s was throttled; wait for a fresh Web authentication rate-limit window, then retry", path)
		}
		if rejected {
			if resp.StatusCode != http.StatusForbidden || resp.Header.Get("Cache-Control") != "no-store" {
				return fmt.Errorf("%s: expected an uncacheable HTTP 403, got HTTP %d", path, resp.StatusCode)
			}
		} else if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login?error=Invalid+email+or+password." {
			return fmt.Errorf("valid token did not reach normal login validation; got HTTP %d", resp.StatusCode)
		}
		return nil
	}
	// Check login first: an unprotected target fails before other endpoints.
	// No probe includes a session, credentials, or account/family form data.
	for _, group := range postGroups {
		for _, route := range group.routes {
			path := strings.ReplaceAll(strings.ReplaceAll(route, ":criterion_id", "00000000-0000-4000-8000-000000000002"), ":id", "00000000-0000-4000-8000-000000000001")
			for _, token := range []string{"", "invalid-token"} {
				if err := post(client, path, token, base, "", true); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintf(out, "PASS: %s forms reject missing and invalid tokens.\n", group.name); err != nil {
			return err
		}
	}
	other, err := newClient()
	if err != nil {
		return err
	}
	if err := post(other, "/login", first, base, "", true); err != nil {
		return fmt.Errorf("missing CSRF cookie: %w", err)
	}
	otherToken, err := load(other)
	if err != nil {
		return err
	}
	if err := post(client, "/login", otherToken, base, "", true); err != nil {
		return fmt.Errorf("token from another browser: %w", err)
	}
	if _, err := fmt.Fprintln(out, "PASS: missing cookies and tokens from another browser are rejected."); err != nil {
		return err
	}
	foreign := u.Scheme + "://csrf-review.invalid"
	sibling := u.Scheme + "://sibling." + u.Hostname()
	if strings.Contains(u.Hostname(), ":") {
		sibling = u.Scheme + "://sibling.example.invalid"
	}
	for _, origin := range []string{foreign, sibling, "null"} {
		if err := post(client, "/login", first, origin, "", true); err != nil {
			return fmt.Errorf("foreign/sibling origin: %w", err)
		}
	}
	if err := post(client, "/login", first, "", foreign+"/login", true); err != nil {
		return fmt.Errorf("foreign Referer: %w", err)
	}
	if _, err := fmt.Fprintln(out, "PASS: foreign, sibling, and null origins and foreign Referer are rejected."); err != nil {
		return err
	}
	for _, token := range []string{first, second} {
		if err := post(client, "/login", token, base, "", false); err != nil {
			return err
		}
	}
	if err := post(client, "/login", first, "", base+"/login", false); err != nil {
		return fmt.Errorf("same-origin Referer fallback: %w", err)
	}
	if _, err := fmt.Fprintln(out, "PASS: tokens from both tabs and same-origin Referer reach normal login validation."); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "PASS: CSRF reviewer check completed with signed-out, empty-form probes.\nSKIP: authenticated actions and backend side effects (run the TestCSRF regression suite).")
	return err
}

func request(client *http.Client, req *http.Request) (*http.Response, string, error) {
	response, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	for _, cookie := range response.Cookies() {
		if cookie.Name != "ournezt_csrf" && cookie.Name != "__Host-ournezt_csrf" {
			return nil, "", fmt.Errorf("signed-out probe unexpectedly changed an application cookie; stopping")
		}
	}
	const limit = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > limit {
		return nil, "", fmt.Errorf("response exceeded 2 MiB")
	}
	return response, string(body), nil
}
