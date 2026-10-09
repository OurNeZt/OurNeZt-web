// auth-rate-check tests login throttling on a running Web + Core deployment.
// Use an isolated local/staging instance with fresh limits. No accounts are
// created. An optional existing test email exercises password verification;
// requests use a generated, deliberately wrong password.
//
// From the Web repository:
//
//	go run ./cmd/auth-rate-check -url http://localhost:8080 -mode account
//	go run ./cmd/auth-rate-check -url http://localhost:8080 -mode ip
//
// Restart Web and Core between runs to give each check a fresh IP budget.
// The IP check rotates X-Forwarded-For and X-Real-IP: run it through the real
// ingress, or directly with WEB_TRUSTED_PROXIES empty.
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
	"strconv"
	"strings"
	"time"
)

type settings struct {
	baseURL  string
	mode     string
	email    string
	limit    int
	recovery bool
	maxWait  time.Duration
}

func main() {
	var cfg settings
	var accountLimit, ipLimit int
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8080", "Web base URL; target an isolated local/staging deployment")
	flag.StringVar(&cfg.mode, "mode", "account", "Check account or ip throttling; restart Web and Core between modes")
	flag.StringVar(&cfg.email, "email", "", "Optional existing staging account for account mode (wrong password only)")
	flag.IntVar(&accountLimit, "account-limit", 5, "Expected AUTH_ACCOUNT_LIMIT for account mode")
	flag.IntVar(&ipLimit, "ip-limit", 30, "Expected Web AUTH_IP_LIMIT for ip mode")
	flag.BoolVar(&cfg.recovery, "recovery", true, "Wait for Retry-After and verify login attempts resume")
	flag.DurationVar(&cfg.maxWait, "max-wait", 90*time.Second, "Maximum allowed recovery wait")
	flag.Parse()
	if cfg.mode == "account" {
		cfg.limit = accountLimit
	} else {
		cfg.limit = ipLimit
	}
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg settings, out io.Writer) error {
	if cfg.mode != "account" && cfg.mode != "ip" {
		return fmt.Errorf("mode must be account or ip")
	}
	if cfg.limit < 1 || cfg.limit > 500 {
		return fmt.Errorf("expected limit must be between 1 and 500")
	}
	base, err := url.Parse(cfg.baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || (base.Path != "" && base.Path != "/") || base.RawQuery != "" || base.Fragment != "" {
		return fmt.Errorf("url must be an HTTP(S) origin without credentials, a path, query, or fragment")
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	nonce := hex.EncodeToString(id)
	email := strings.ToLower(strings.TrimSpace(cfg.email))
	if email == "" {
		email = "rate-check-" + nonce + "@example.invalid"
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	loginURL := strings.TrimRight(cfg.baseURL, "/") + "/login"
	attempt := func(email string, spoof int) (*http.Response, error) {
		form := url.Values{"email": {email}, "password": {"deliberately-wrong-" + nonce}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if spoof > 0 {
			address := fmt.Sprintf("198.18.%d.%d", spoof/256, spoof%256)
			req.Header.Set("X-Forwarded-For", address)
			req.Header.Set("X-Real-IP", address)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("login request failed: %w", err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		if err := resp.Body.Close(); err != nil {
			return nil, err
		}
		return resp, nil
	}
	emailFor := func(i int) string {
		if cfg.mode == "ip" {
			return fmt.Sprintf("rate-check-%s-%d@example.invalid", nonce, i)
		}
		return email
	}
	for i := 1; i <= cfg.limit; i++ {
		spoof := 0
		if cfg.mode == "ip" {
			spoof = i
		}
		resp, err := attempt(emailFor(i), spoof)
		if err != nil {
			return err
		}
		if err := expectCredentialFailure(resp); err != nil {
			return fmt.Errorf("attempt %d of %d: %w; use fresh limits and matching configuration", i, cfg.limit, err)
		}
	}
	if _, err := fmt.Fprintf(out, "PASS: first %d requests returned credential-failure responses.\n", cfg.limit); err != nil {
		return err
	}
	spoof := 0
	if cfg.mode == "ip" {
		spoof = cfg.limit + 1
	}
	resp, err := attempt(emailFor(cfg.limit+1), spoof)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("request %d returned HTTP %d; expected 429", cfg.limit+1, resp.StatusCode)
	}
	retry, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || retry < 1 || retry > 86400 {
		return fmt.Errorf("throttled response needs a positive Retry-After of at most one day")
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		return fmt.Errorf("throttled response must use Cache-Control: no-store")
	}
	if _, err := fmt.Fprintf(out, "PASS: request %d was throttled with HTTP 429 and Retry-After.\n", cfg.limit+1); err != nil {
		return err
	}
	if cfg.mode == "account" {
		resp, err = attempt(" "+strings.ToUpper(email)+" ", 0)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return fmt.Errorf("email case/whitespace change bypassed account throttle: HTTP %d", resp.StatusCode)
		}
		if _, err := fmt.Fprintln(out, "PASS: email case and whitespace cannot bypass the account limit."); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(out, "PASS: rotating emails and spoofed forwarding headers cannot bypass the IP limit."); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return err
	}
	page, err := client.Do(req)
	if err != nil {
		return err
	}
	_, readErr := io.Copy(io.Discard, page.Body)
	closeErr := page.Body.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if page.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /login returned HTTP %d while POSTs were throttled", page.StatusCode)
	}
	if _, err := fmt.Fprintln(out, "PASS: the login page remains available while submissions are throttled."); err != nil {
		return err
	}
	if cfg.recovery {
		wait := time.Duration(retry+1) * time.Second
		if wait > cfg.maxWait {
			return fmt.Errorf("recovery requires %s, exceeding max-wait %s; increase -max-wait or use -recovery=false", wait, cfg.maxWait)
		}
		if _, err := fmt.Fprintf(out, "Waiting %s to check recovery...\n", wait); err != nil {
			return err
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		resp, err = attempt(emailFor(cfg.limit+2), 0)
		if err != nil {
			return err
		}
		if err := expectCredentialFailure(resp); err != nil {
			return fmt.Errorf("after retry window: %w", err)
		}
		if _, err := fmt.Fprintln(out, "PASS: attempts resume after the retry window."); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, "PASS: authentication rate-limit check completed.")
	return err
}

func expectCredentialFailure(resp *http.Response) error {
	if resp.StatusCode != http.StatusFound {
		return fmt.Errorf("expected failed-login redirect, got HTTP %d", resp.StatusCode)
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || location.Path != "/login" {
		return fmt.Errorf("expected redirect to /login")
	}
	// Accept old credential errors so an unfixed app reaches the missing-429
	// assertion. Service outages and successful logins must fail the check.
	switch location.Query().Get("error") {
	case "Invalid email or password.", "not found", "unauthenticated", "disabled user":
		return nil
	default:
		return fmt.Errorf("response did not indicate a credential failure; ensure Core is reachable")
	}
}
