package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	pb "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/OurNeZt/ournezt-web/internal/web"
	"google.golang.org/grpc"
)

type reviewerAuthClient struct {
	pb.AuthServiceClient
	calls atomic.Int32
}

func (f *reviewerAuthClient) Login(context.Context, *pb.LoginRequest, ...grpc.CallOption) (*pb.LoginResponse, error) {
	f.calls.Add(1)
	return nil, errors.New("reviewer probes must not reach Core")
}

func TestHTTPReviewerAgainstApplication(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, limit := range []int64{config.DefaultMaxRequestBodyBytes, 128} {
		t.Run(fmtLimit(limit), func(t *testing.T) {
			client := &reviewerAuthClient{}
			cfg := config.Config{MaxRequestBodyBytes: limit}
			router, err := web.NewRouter(cfg, &core.Clients{Auth: client})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(router)
			server.Config = web.NewHTTPServer(cfg, router)
			server.Start()
			defer server.Close()
			var out bytes.Buffer
			err = run(context.Background(), settings{baseURL: server.URL, bodyLimit: limit, timeout: time.Second}, &out)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(out.String(), "PASS:") != 9 {
				t.Fatalf("checks incomplete: %s", out.String())
			}
			if client.calls.Load() != 0 {
				t.Fatalf("reviewer check reached Core %d times", client.calls.Load())
			}
		})
	}
}

func fmtLimit(limit int64) string {
	if limit == config.DefaultMaxRequestBodyBytes {
		return "defaults"
	}
	return "custom global cap below auth cap"
}

func TestHTTPReviewerDetectsUnfixedOrUnavailableEndpoint(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		err := run(context.Background(), settings{baseURL: server.URL, bodyLimit: 128, timeout: time.Second}, &bytes.Buffer{})
		server.Close()
		if err == nil {
			t.Fatalf("endpoint always returning %d falsely passed", code)
		}
	}
}

func TestHTTPReviewerDetectsMissingChunkedProtection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.ContentLength > 128 {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	err := run(context.Background(), settings{baseURL: server.URL, bodyLimit: 128, timeout: time.Second}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "oversized chunked body") {
		t.Fatalf("Content-Length-only protection falsely passed: %v", err)
	}
}

func TestHTTPReviewerActuallySendsChunkedBody(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 129)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != -1 || len(r.TransferEncoding) != 1 || r.TransferEncoding[0] != "chunked" {
			t.Errorf("request was not chunked: length=%d, encoding=%v", r.ContentLength, r.TransferEncoding)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, payload) {
			t.Errorf("unexpected payload: length=%d, error=%v", len(body), err)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	}))
	defer server.Close()
	err := check(context.Background(), server.Client(), server.URL, probe{name: "chunked", path: "/", body: payload, chunked: true, status: http.StatusRequestEntityTooLarge})
	if err != nil {
		t.Fatal(err)
	}
}

type brokenProgressWriter struct{}

func (brokenProgressWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestHTTPReviewerStopsOnOutputError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	err := run(context.Background(), settings{baseURL: server.URL, bodyLimit: 128, timeout: time.Second}, brokenProgressWriter{})
	if !errors.Is(err, io.ErrClosedPipe) || calls.Load() != 2 {
		t.Fatalf("output failure ignored: %v, %d requests", err, calls.Load())
	}
}

func TestHTTPReviewerRequiresNonCacheableRejections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusRequestEntityTooLarge) }))
	defer server.Close()
	err := check(context.Background(), server.Client(), server.URL, probe{name: "oversized", path: "/", status: http.StatusRequestEntityTooLarge})
	if err == nil || !strings.Contains(err.Error(), "no-store") {
		t.Fatalf("cacheable rejection passed: %v", err)
	}
}

func TestHTTPReviewerValidatesSettings(t *testing.T) {
	for _, cfg := range []settings{
		{baseURL: "file:///tmp/test", bodyLimit: 128, timeout: time.Second},
		{baseURL: "http://user:password@localhost", bodyLimit: 128, timeout: time.Second},
		{baseURL: "http://localhost/path", bodyLimit: 128, timeout: time.Second},
		{baseURL: "http://localhost?query=1", bodyLimit: 128, timeout: time.Second},
		{baseURL: "http://localhost#fragment", bodyLimit: 128, timeout: time.Second},
		{baseURL: "http://localhost", bodyLimit: 0, timeout: time.Second},
		{baseURL: "http://localhost", bodyLimit: 1 << 30, timeout: time.Second},
		{baseURL: "http://localhost", bodyLimit: 128, timeout: 0},
	} {
		if err := run(context.Background(), cfg, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid settings accepted: %+v", cfg)
		}
	}
}
