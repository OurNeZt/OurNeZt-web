package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func transportCertificate(t *testing.T) (string, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), DNSNames: []string{"core.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certificate, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(file, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return file, certificate
}

func transportServer(t *testing.T, certificate *tls.Certificate) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	options := []grpc.ServerOption{grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		remote, _ := peer.FromContext(ctx)
		if remote == nil {
			t.Error("missing transport peer")
		} else {
			info, tlsUsed := remote.AuthInfo.(credentials.TLSInfo)
			if tlsUsed != (certificate != nil) || tlsUsed && info.State.Version < tls.VersionTLS12 {
				t.Error("session metadata reached an unexpected transport")
			}
		}
		md, _ := metadata.FromIncomingContext(ctx)
		if tokens := md.Get("x-session-token"); len(tokens) != 1 || tokens[0] != "transport-test-token" {
			t.Error("session metadata changed")
		}
		return next(ctx, req)
	})}
	if certificate != nil {
		options = append(options, grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*certificate}})))
	}
	transport := grpc.NewServer(options...)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(transport, healthServer)
	go func() { _ = transport.Serve(listener) }()
	t.Cleanup(transport.Stop)
	return listener.Addr().String(), &calls
}

func TestTransportClientHandshakesAndSessionMetadata(t *testing.T) {
	caFile, certificate := transportCertificate(t)
	otherCA, _ := transportCertificate(t)
	tlsAddress, tlsCalls := transportServer(t, &certificate)
	plainAddress, plainCalls := transportServer(t, nil)
	for _, tc := range []struct {
		name, environment, address, ca, serverName, warning string
		useTLS, allow, skip, success                        bool
	}{
		{"verified production TLS", "production", tlsAddress, caFile, "core.test", "", true, false, false, true},
		{"opt-in retains TLS", "production", tlsAddress, caFile, "core.test", "", true, true, false, true},
		{"wrong server name", "production", tlsAddress, caFile, "wrong.test", "", true, false, false, false},
		{"untrusted CA", "production", tlsAddress, otherCA, "core.test", "", true, false, false, false},
		{"system roots reject self-signed", "production", tlsAddress, "", "core.test", "", true, false, false, false},
		{"TLS never downgrades", "production", plainAddress, caFile, "core.test", "", true, true, false, false},
		{"explicit production plaintext", "production", plainAddress, "", "", "CORE_ALLOW_INSECURE", false, true, false, true},
		{"development default", "development", plainAddress, "", "", "", false, false, false, true},
		{"development verification bypass", "development", tlsAddress, "", "wrong.test", "CORE_GRPC_TLS_INSECURE_SKIP_VERIFY", true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cfg := config.Config{AppEnv: tc.environment, CoreGRPCAddr: tc.address, CoreGRPCUseTLS: tc.useTLS, CoreAllowInsecure: tc.allow, CoreGRPCInsecure: tc.skip, CoreGRPCTLSCAFile: tc.ca, CoreGRPCTLSServer: tc.serverName, RequestTimeout: time.Second}
			clients, err := NewClients(context.Background(), cfg, slog.New(slog.NewJSONHandler(&output, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = clients.Close() }()
			counter := tlsCalls
			if tc.address == plainAddress {
				counter = plainCalls
			}
			before := counter.Load()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(ctx, "x-session-token", "transport-test-token")
			_, err = grpc_health_v1.NewHealthClient(clients.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			if (err == nil) != tc.success {
				t.Fatalf("RPC success=%t error=%v", tc.success, err)
			}
			wantCalls := int32(0)
			if tc.success {
				wantCalls = 1
			}
			if counter.Load()-before != wantCalls {
				t.Fatal("failed TLS handshake delivered session metadata to an RPC handler")
			}
			if tc.warning == "" && output.Len() != 0 || tc.warning != "" && (!strings.Contains(output.String(), tc.warning) || !strings.Contains(output.String(), `"level":"WARN"`)) {
				t.Fatalf("incorrect transport warning: %s", output.String())
			}
		})
	}
}

func TestTransportClientRejectsUnsafePolicyAndMaterial(t *testing.T) {
	badCA := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(badCA, []byte("not a PEM certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []config.Config{
		{AppEnv: "production"},
		{AppEnv: "production", CoreAllowInsecure: true, CoreGRPCUseTLS: true, CoreGRPCInsecure: true},
		{AppEnv: "production", CoreAllowInsecure: true, CoreGRPCTLSCAFile: badCA},
		{AppEnv: "production", CoreAllowInsecure: true, CoreGRPCUseTLS: true, CoreGRPCTLSCAFile: badCA},
		{AppEnv: "production", CoreAllowInsecure: true, CoreGRPCUseTLS: true, CoreGRPCTLSCAFile: badCA + ".missing"},
	} {
		cfg.CoreGRPCAddr = "127.0.0.1:1"
		var output bytes.Buffer
		clients, err := NewClients(context.Background(), cfg, slog.New(slog.NewJSONHandler(&output, nil)))
		if err == nil || clients != nil || output.Len() != 0 {
			t.Fatalf("unsafe configuration created clients or enabled a fallback: clients=%v error=%v logs=%s", clients, err, output.String())
		}
	}
}
