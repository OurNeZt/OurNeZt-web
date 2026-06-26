package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv            string
	WebAddr           string
	WebTLSCertFile    string
	WebTLSKeyFile     string
	CoreGRPCAddr      string
	CoreGRPCUseTLS    bool
	CoreGRPCTLSCAFile string
	CoreGRPCTLSServer string
	CoreGRPCInsecure  bool
	ShutdownTimeout   time.Duration
	RequestTimeout    time.Duration
	SessionCookieName string
	SessionCookieMax  time.Duration
	CookieSecure      bool
	MaintenanceNotice MaintenanceNoticeConfig
}

type MaintenanceNoticeConfig struct {
	FilePath string
	Enabled  bool
	Level    string
	Title    string
	Message  string
	StartsAt string
	EndsAt   string
}

func Load() Config {
	return Config{
		AppEnv:            env("APP_ENV", "development"),
		WebAddr:           env("WEB_ADDR", ":8080"),
		WebTLSCertFile:    env("WEB_TLS_CERT_FILE", ""),
		WebTLSKeyFile:     env("WEB_TLS_KEY_FILE", ""),
		CoreGRPCAddr:      env("CORE_GRPC_ADDR", "localhost:50051"),
		CoreGRPCUseTLS:    envBool("CORE_GRPC_USE_TLS", false),
		CoreGRPCTLSCAFile: env("CORE_GRPC_TLS_CA_FILE", ""),
		CoreGRPCTLSServer: env("CORE_GRPC_TLS_SERVER_NAME", ""),
		CoreGRPCInsecure:  envBool("CORE_GRPC_TLS_INSECURE_SKIP_VERIFY", false),
		ShutdownTimeout:   envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:    envDuration("REQUEST_TIMEOUT", 5*time.Second),
		SessionCookieName: env("SESSION_COOKIE_NAME", "ournezt_session"),
		SessionCookieMax:  envDuration("SESSION_COOKIE_MAX_AGE", 24*time.Hour),
		CookieSecure:      envBool("SESSION_COOKIE_SECURE", defaultCookieSecure(strings.EqualFold(env("APP_ENV", "development"), "production"))),
		MaintenanceNotice: MaintenanceNoticeConfig{
			FilePath: env("MAINTENANCE_NOTICE_FILE", ""),
			Enabled:  envBool("MAINTENANCE_NOTICE_ENABLED", false),
			Level:    env("MAINTENANCE_NOTICE_LEVEL", "warning"),
			Title:    env("MAINTENANCE_NOTICE_TITLE", ""),
			Message:  env("MAINTENANCE_NOTICE_MESSAGE", ""),
			StartsAt: env("MAINTENANCE_NOTICE_STARTS_AT", ""),
			EndsAt:   env("MAINTENANCE_NOTICE_ENDS_AT", ""),
		},
	}
}

func defaultCookieSecure(isProduction bool) bool {
	return isProduction
}

func (c Config) LogLevel() slog.Level {
	if strings.EqualFold(c.AppEnv, "development") {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
