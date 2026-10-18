// Package checkclient provides browser-like CSRF handling for reviewer commands.
package checkclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

// PrimeCSRF loads the login form and retains its signed cookie. An empty token
// supports checks against older deployments that do not yet implement CSRF.
func PrimeCSRF(ctx context.Context, client *http.Client, base string) (string, error) {
	if client.Jar == nil {
		jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		if err != nil {
			return "", err
		}
		client.Jar = jar
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/login", nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	const limit = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", err
	}
	if len(body) > limit {
		return "", fmt.Errorf("login form exceeded 2 MiB")
	}
	// Older check targets may have no login page. Let the original probes
	// report their own failures rather than treating a missing token as success.
	return CSRFToken(string(body)), nil
}

// CSRFToken reads only the hidden form field, never the cookie's secret seed.
func CSRFToken(page string) string {
	z := html.NewTokenizer(strings.NewReader(page))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data != "input" {
				continue
			}
			var name, value, kind string
			for _, attr := range t.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				case "type":
					kind = attr.Val
				}
			}
			if name == "csrf_token" && kind == "hidden" {
				return value
			}
		}
	}
}

var csrfInput = regexp.MustCompile(`<input type="hidden" name="csrf_token" value="[^"]*">`)

// NormalizeCSRF removes only the per-request masked token when comparing login
// failure pages. All other response bytes still have to match.
func NormalizeCSRF(page string) string {
	return csrfInput.ReplaceAllString(page, `<input type="hidden" name="csrf_token" value="[csrf]">`)
}
