package config

import (
	"testing"
	"time"
)

func TestHTTPProtectionConfiguration(t *testing.T) {
	keys := []string{"WEB_READ_HEADER_TIMEOUT", "WEB_READ_TIMEOUT", "WEB_WRITE_TIMEOUT", "WEB_IDLE_TIMEOUT", "WEB_MAX_REQUEST_BODY_BYTES"}
	for _, key := range keys {
		t.Setenv(key, "")
	}
	checkDefaults := func(t *testing.T, cfg Config) {
		t.Helper()
		if cfg.ReadHeaderTimeout != 10*time.Second || cfg.ReadTimeout != 15*time.Second || cfg.WriteTimeout != 30*time.Second || cfg.IdleTimeout != time.Minute || cfg.MaxRequestBodyBytes != 1<<20 {
			t.Fatalf("HTTP protections are not enabled by default: %+v", cfg)
		}
	}
	t.Run("defaults", func(t *testing.T) { checkDefaults(t, Load()) })
	t.Run("overrides", func(t *testing.T) {
		for i, value := range []string{"2s", "8s", "12s", "20s", "2097152"} {
			t.Setenv(keys[i], value)
		}
		cfg := Load()
		if cfg.ReadHeaderTimeout != 2*time.Second || cfg.ReadTimeout != 8*time.Second || cfg.WriteTimeout != 12*time.Second || cfg.IdleTimeout != 20*time.Second || cfg.MaxRequestBodyBytes != 2<<20 {
			t.Fatalf("HTTP overrides were not loaded: %+v", cfg)
		}
	})
	for _, invalid := range []string{"0", "-1", "invalid", "999999999999999999999999999999"} {
		t.Run("invalid "+invalid, func(t *testing.T) {
			for _, key := range keys {
				t.Setenv(key, invalid)
			}
			checkDefaults(t, Load())
		})
	}
	t.Run("nonpositive durations", func(t *testing.T) {
		for _, key := range keys[:4] {
			t.Setenv(key, "-1s")
		}
		checkDefaults(t, Load())
	})
}
