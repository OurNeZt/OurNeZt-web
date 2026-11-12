package config

import (
	"strings"
	"testing"
)

func TestTransportProductionConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, environment, allow, tls, skip, ca, server, want string
	}{
		{"development default", "development", "", "", "", "", "", ""},
		{"production default", "production", "", "", "", "", "", "CORE_GRPC_USE_TLS=true"},
		{"production normalized", " PRODUCTION ", "false", "false", "false", "", "", "production requires"},
		{"invalid opt-in fails closed", "production", "tru", "false", "false", "", "", "CORE_ALLOW_INSECURE must be a boolean"},
		{"invalid TLS flag fails closed", "production", "false", "tru", "false", "", "", "CORE_GRPC_USE_TLS must be a boolean"},
		{"opt-in cannot hide invalid TLS flag", "production", "true", "tru", "false", "", "", "CORE_GRPC_USE_TLS must be a boolean"},
		{"invalid skip flag fails closed", "production", "false", "true", "tru", "", "", "CORE_GRPC_TLS_INSECURE_SKIP_VERIFY must be a boolean"},
		{"explicit production plaintext", "production", "true", "false", "false", "", "", ""},
		{"production TLS", "production", "false", "true", "false", "ca.pem", "core.test", ""},
		{"production verification bypass", "production", "false", "true", "true", "", "", "forbidden in production"},
		{"opt-in cannot bypass verification", "production", "true", "true", "true", "", "", "forbidden in production"},
		{"development verification bypass", "development", "false", "true", "true", "", "", ""},
		{"CA would be ignored", "development", "false", "false", "false", "ca.pem", "", "refusing to ignore TLS settings"},
		{"server name would be ignored", "production", "true", "false", "false", "", "core.test", "refusing to ignore TLS settings"},
		{"skip setting would be ignored", "development", "true", "false", "true", "", "", "refusing to ignore TLS settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.environment)
			t.Setenv("CORE_ALLOW_INSECURE", tc.allow)
			t.Setenv("CORE_GRPC_USE_TLS", tc.tls)
			t.Setenv("CORE_GRPC_TLS_INSECURE_SKIP_VERIFY", tc.skip)
			t.Setenv("CORE_GRPC_TLS_CA_FILE", tc.ca)
			t.Setenv("CORE_GRPC_TLS_SERVER_NAME", tc.server)
			cfg := Load()
			err := cfg.ValidateCoreTransport()
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("ValidateCoreTransport = %v, want %q", err, tc.want)
			}
			if cfg.CoreAllowInsecure != (tc.allow == "true") {
				t.Fatal("insecure transport was not an explicit opt-in")
			}
		})
	}
}
