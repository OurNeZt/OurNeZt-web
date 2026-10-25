package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTransportStartupHelper(t *testing.T) {
	if os.Getenv("OURNEZT_TEST_TRANSPORT_STARTUP") == "1" {
		main()
	}
}

func TestTransportStartupRejectsBeforeHTTP(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, allow, tls, skip, ca, want string }{
		{"missing TLS", "false", "false", "false", "", "production requires CORE_GRPC_USE_TLS=true"},
		{"invalid TLS flag with opt-in", "true", "tru", "false", "", "CORE_GRPC_USE_TLS must be a boolean"},
		{"ignored TLS settings", "true", "false", "false", "missing-ca.pem", "refusing to ignore TLS settings"},
		{"bad CA with opt-in", "true", "true", "false", "missing-ca.pem", "read core grpc CA file"},
		{"verification bypass with opt-in", "true", "true", "true", "", "forbidden in production"},
		{"trusted plaintext warning", "true", "false", "false", "", "plaintext Core gRPC explicitly enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestTransportStartupHelper$")
			command.Env = append(os.Environ(), "OURNEZT_TEST_TRANSPORT_STARTUP=1", "APP_ENV=production", "CSRF_AUTH_KEY=", "CORE_GRPC_ADDR=127.0.0.1:1", "CORE_ALLOW_INSECURE="+tc.allow, "CORE_GRPC_USE_TLS="+tc.tls, "CORE_GRPC_TLS_INSECURE_SKIP_VERIFY="+tc.skip, "CORE_GRPC_TLS_CA_FILE="+tc.ca, "CORE_GRPC_TLS_SERVER_NAME=")
			output, err := command.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 1 || !strings.Contains(string(output), tc.want) {
				t.Fatalf("unexpected startup exit: err=%v logs=%s", err, output)
			}
			if tc.allow == "false" || tc.ca != "" || tc.skip != "false" || tc.tls != "false" {
				if strings.Contains(string(output), "build router") || strings.Contains(string(output), "ournezt web started") {
					t.Fatal("invalid Core transport reached HTTP startup")
				}
			} else if !strings.Contains(string(output), "build router") || !strings.Contains(string(output), `"level":"WARN"`) {
				t.Fatal("explicit plaintext did not pass policy with a clear warning")
			}
		})
	}
}
