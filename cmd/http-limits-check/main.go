// http-limits-check verifies request body protections on a running Web instance.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/checkclient"
	"github.com/OurNeZt/ournezt-web/internal/config"
)

type settings struct {
	baseURL   string
	bodyLimit int64
	timeout   time.Duration
}

type probe struct {
	csrfToken               string
	name, path, contentType string
	body                    []byte
	chunked                 bool
	status                  int
}

func main() {
	var cfg settings
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8080", "Web origin (use a direct local or staging listener)")
	flag.Int64Var(&cfg.bodyLimit, "max-body-bytes", config.DefaultMaxRequestBodyBytes, "expected WEB_MAX_REQUEST_BODY_BYTES (64 bytes to 64 MiB)")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "timeout for each probe")
	flag.Parse()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg settings, out io.Writer) error {
	origin, err := url.Parse(cfg.baseURL)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("-url must be an HTTP(S) origin without credentials, a path, query, or fragment")
	}
	if cfg.bodyLimit < 64 || cfg.bodyLimit > 64<<20 {
		return fmt.Errorf("-max-body-bytes must be between 64 and 67108864")
	}
	if cfg.timeout <= 0 {
		return fmt.Errorf("-timeout must be positive")
	}
	base := strings.TrimSuffix(origin.String(), "/")
	client := &http.Client{
		Timeout:       cfg.timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	token, err := checkclient.PrimeCSRF(ctx, client, base)
	if err != nil {
		return fmt.Errorf("load login form: %w", err)
	}
	// This deliberately unregistered path cannot submit an application form.
	const path = "/__ournezt_http_limits_check__"
	payload := bytes.Repeat([]byte("x"), int(cfg.bodyLimit)+1)
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, err := writer.CreateFormFile("unused", "probe.txt")
	if err != nil {
		return err
	}
	if _, err := part.Write(payload); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	authLimit := min(cfg.bodyLimit, int64(16<<10))
	probes := []probe{
		{name: "unregistered path baseline", path: path, status: http.StatusNotFound},
		{name: "body exactly at global limit", path: path, body: payload[:cfg.bodyLimit], status: http.StatusNotFound},
		{name: "oversized Content-Length body", path: path, body: payload, status: http.StatusRequestEntityTooLarge},
		{name: "oversized chunked body", path: path, body: payload, chunked: true, status: http.StatusRequestEntityTooLarge},
		{name: "oversized chunked multipart form", path: path, body: multipartBody.Bytes(), contentType: writer.FormDataContentType(), chunked: true, status: http.StatusRequestEntityTooLarge},
		{name: "malformed URL-encoded form", path: path, body: []byte("name=valid&bad=%zz"), contentType: "application/x-www-form-urlencoded", status: http.StatusBadRequest},
		{name: "malformed multipart form", path: path, body: []byte("missing multipart boundary"), contentType: "multipart/form-data", status: http.StatusBadRequest},
		{name: "smaller authentication body cap", path: "/login", body: payload[:authLimit+1], contentType: "application/x-www-form-urlencoded", status: http.StatusRequestEntityTooLarge},
	}
	for _, p := range probes {
		p.csrfToken = token
		if err := check(ctx, client, base, p); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "PASS: %s (HTTP %d)\n", p.name, p.status); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, "PASS: all request body checks; run the HTTP server regression tests for connection timeouts.")
	return err
}

func check(ctx context.Context, client *http.Client, base string, p probe) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+p.path, bytes.NewReader(p.body))
	if err != nil {
		return err
	}
	if p.chunked {
		req.ContentLength = -1
	}
	req.Header.Set("Origin", base)
	req.Header.Set("X-CSRF-Token", p.csrfToken)
	if p.contentType != "" {
		req.Header.Set("Content-Type", p.contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != p.status {
		return fmt.Errorf("%s: expected HTTP %d, got HTTP %d (check the target, configured limits, and existing authentication throttling)", p.name, p.status, resp.StatusCode)
	}
	if p.status == http.StatusRequestEntityTooLarge || p.status == http.StatusBadRequest {
		if resp.Header.Get("Cache-Control") != "no-store" {
			return fmt.Errorf("%s: expected Cache-Control: no-store", p.name)
		}
	}
	// Bound checker response reads as well, even if the target is misconfigured.
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)); err != nil {
		return fmt.Errorf("%s: read response: %w", p.name, err)
	}
	return nil
}
