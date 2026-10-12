package web

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/gin-gonic/gin"
)

func startHTTPProtectionServer(t *testing.T, cfg config.Config, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.Config = NewHTTPServer(cfg, handler)
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func protectionConnection(t *testing.T, server *httptest.Server) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(server.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestHTTPServerClosesSlowHeaders(t *testing.T) {
	var called atomic.Bool
	server := startHTTPProtectionServer(t, config.Config{ReadHeaderTimeout: 100 * time.Millisecond}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	conn := protectionConnection(t, server)
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost:"); err != nil {
		t.Fatal(err)
	}
	_, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil || called.Load() {
		t.Fatal("incomplete headers reached the handler")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("connection only closed at the client's deadline")
	}
}

func TestHTTPServerRejectsSlowBody(t *testing.T) {
	var called atomic.Bool
	router := bodyLimitRouter(config.Config{}, func(c *gin.Context) { called.Store(true); c.Status(http.StatusNoContent) })
	server := startHTTPProtectionServer(t, config.Config{ReadTimeout: 100 * time.Millisecond, WriteTimeout: time.Second}, router)
	conn := protectionConnection(t, server)
	if _, err := io.WriteString(conn, "POST /submit HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nname=a"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusRequestTimeout || called.Load() {
		t.Fatalf("slow body processed: %d", response.StatusCode)
	}
}

func TestHTTPServerRejectsOversizedChunkedBodyPromptly(t *testing.T) {
	var called atomic.Bool
	router := bodyLimitRouter(config.Config{MaxRequestBodyBytes: 8}, func(c *gin.Context) { called.Store(true); c.Status(http.StatusNoContent) })
	server := startHTTPProtectionServer(t, config.Config{ReadTimeout: 5 * time.Second}, router)
	conn := protectionConnection(t, server)
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// Do not terminate the chunked stream. Rejection must not wait for EOF or
	// for the server's five-second body-read timeout before replying.
	if _, err := io.WriteString(conn, "POST /submit HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n9\r\n123456789\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusRequestEntityTooLarge || called.Load() {
		t.Fatalf("oversized chunked request processed: %d", response.StatusCode)
	}
}

func TestHTTPServerClosesIdleKeepAlive(t *testing.T) {
	server := startHTTPProtectionServer(t, config.Config{IdleTimeout: 100 * time.Millisecond}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	conn := protectionConnection(t, server)
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent || response.Close {
		t.Fatal("initial keep-alive request failed")
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatalf("idle connection was not closed by the server: %v", err)
	}
}

func TestHTTPServerBoundsResponseWrites(t *testing.T) {
	server := startHTTPProtectionServer(t, config.Config{WriteTimeout: 100 * time.Millisecond}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_, _ = io.WriteString(w, "late response")
	}))
	conn := protectionConnection(t, server)
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := http.ReadResponse(bufio.NewReader(conn), nil); !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("response write deadline was not enforced: %v", err)
	}
}

func TestHTTPServerBoundsTLSBodies(t *testing.T) {
	var called atomic.Bool
	router := bodyLimitRouter(config.Config{MaxRequestBodyBytes: 64}, func(c *gin.Context) { called.Store(true); c.Status(http.StatusNoContent) })
	server := httptest.NewUnstartedServer(router)
	server.Config = NewHTTPServer(config.Config{}, router)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 3 * time.Second
	for _, unknownLength := range []bool{false, true} {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/submit", strings.NewReader(strings.Repeat("x", 65)))
		if err != nil {
			t.Fatal(err)
		}
		if unknownLength {
			req.ContentLength = -1
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusRequestEntityTooLarge || response.ProtoMajor != 2 || called.Load() {
			t.Errorf("HTTP/2 body limit bypassed: %s %d", response.Proto, response.StatusCode)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
