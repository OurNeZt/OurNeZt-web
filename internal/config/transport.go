package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.AppEnv), "production")
}

// ValidateCoreTransport is also called by the client constructor, so callers
// cannot bypass the production policy by constructing a Config directly.
func (c Config) ValidateCoreTransport() error {
	if c.transportError != nil {
		return c.transportError
	}
	if c.IsProduction() && c.CoreGRPCInsecure {
		return fmt.Errorf("CORE_GRPC_TLS_INSECURE_SKIP_VERIFY is forbidden in production; configure a trusted CA and matching server name")
	}
	if !c.CoreGRPCUseTLS {
		if strings.TrimSpace(c.CoreGRPCTLSCAFile) != "" || strings.TrimSpace(c.CoreGRPCTLSServer) != "" || c.CoreGRPCInsecure {
			return fmt.Errorf("CORE_GRPC_USE_TLS=true is required when Core TLS settings are configured; refusing to ignore TLS settings and use plaintext")
		}
		if c.IsProduction() && !c.CoreAllowInsecure {
			return fmt.Errorf("production requires CORE_GRPC_USE_TLS=true; set CORE_ALLOW_INSECURE=true only for a trusted internal network or TLS-terminating proxy")
		}
	}
	return nil
}

func transportBooleanError(keys ...string) error {
	for _, key := range keys {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value == "" {
			continue
		}
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%s must be a boolean (true or false)", key)
		}
	}
	return nil
}
