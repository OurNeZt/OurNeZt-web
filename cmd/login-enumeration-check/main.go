// login-enumeration-check compares failed logins on a running Web instance.
// Use existing local/staging fixtures; no accounts are created or updated.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/checkclient"
)

type settings struct {
	baseURL, email, disabledEmail string
	timeout                       time.Duration
}

func main() {
	var cfg settings
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8080", "Web origin for a local/staging deployment")
	flag.StringVar(&cfg.email, "email", "", "Required existing active test account email")
	flag.StringVar(&cfg.disabledEmail, "disabled-email", "", "Optional existing disabled test account email")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "Timeout for each HTTP request")
	flag.Parse()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg settings, out io.Writer) error {
	origin, err := url.Parse(cfg.baseURL)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("-url must be an HTTP(S) origin without credentials, a path, query, or fragment")
	}
	if cfg.timeout <= 0 {
		return fmt.Errorf("-timeout must be positive")
	}
	active := strings.ToLower(strings.TrimSpace(cfg.email))
	disabled := strings.ToLower(strings.TrimSpace(cfg.disabledEmail))
	if active == "" {
		return fmt.Errorf("-email must identify an existing active test account")
	}
	if disabled != "" && disabled == active {
		return fmt.Errorf("active and disabled fixtures must be different accounts")
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	nonce := hex.EncodeToString(random[:])
	password := "deliberately-wrong-" + nonce
	type probe struct{ name, email string }
	probes := []probe{
		{"unknown email", "login-check-" + nonce + "@example.invalid"},
		{"active account with wrong password", active},
	}
	if disabled != "" {
		probes = append(probes, probe{"disabled account", disabled})
	}
	client := &http.Client{Timeout: cfg.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := strings.TrimRight(origin.String(), "/")
	token, err := checkclient.PrimeCSRF(ctx, client, base)
	if err != nil {
		return fmt.Errorf("load login form: %w", err)
	}
	var baselinePost, baselinePage string
	for i, probe := range probes {
		form := url.Values{"email": {probe.email}, "password": {password}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/login", strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", base)
		req.Header.Set("X-CSRF-Token", token)
		response, post, err := request(client, req)
		if err != nil {
			return fmt.Errorf("%s request failed: %w", probe.name, err)
		}
		if response.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("%s was throttled; wait for fresh Web and Core rate-limit budgets, then retry", probe.name)
		}
		const location = "/login?error=Invalid+email+or+password."
		if response.StatusCode != http.StatusFound || response.Header.Get("Location") != location || len(response.Header.Values("Set-Cookie")) != 0 {
			return fmt.Errorf("%s: expected HTTP 302, the generic login redirect, and no Set-Cookie", probe.name)
		}
		pageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+location, nil)
		if err != nil {
			return err
		}
		pageResponse, page, err := request(client, pageReq)
		if err != nil {
			return err
		}
		if pageResponse.StatusCode != http.StatusOK || !strings.Contains(pageResponse.Header.Get("Content-Type"), "text/html") || !strings.Contains(page, "Invalid email or password.") || len(pageResponse.Header.Values("Set-Cookie")) != 0 {
			return fmt.Errorf("%s: expected an HTML login page with the generic message and no Set-Cookie", probe.name)
		}
		page = checkclient.NormalizeCSRF(page)
		if i == 0 {
			baselinePost, baselinePage = post, page
		} else if post != baselinePost || page != baselinePage {
			return fmt.Errorf("%s: failed-login response bodies differ from the unknown-account response", probe.name)
		}
		if _, err := fmt.Fprintf(out, "PASS: %s uses the generic redirect and page; no session cookie.\n", probe.name); err != nil {
			return err
		}
	}
	if disabled == "" {
		if _, err := fmt.Fprintln(out, "SKIP: disabled account (supply -disabled-email to include an existing fixture)."); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, "PASS: Web login responses match. Run Core regression tests to verify backend status codes and dummy Argon2 work.")
	return err
}

func request(client *http.Client, req *http.Request) (*http.Response, string, error) {
	response, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxBody {
		return nil, "", fmt.Errorf("response exceeded 2 MiB")
	}
	return response, string(body), nil
}
