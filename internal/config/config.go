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
	CoreGRPCAddr      string
	ShutdownTimeout   time.Duration
	RequestTimeout    time.Duration
	SessionCookieName string
	SessionCookieMax  time.Duration
	CookieSecure      bool
}

func Load() Config {
	return Config{
		AppEnv:            env("APP_ENV", "development"),
		WebAddr:           env("WEB_ADDR", ":8080"),
		CoreGRPCAddr:      env("CORE_GRPC_ADDR", "localhost:50051"),
		ShutdownTimeout:   envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:    envDuration("REQUEST_TIMEOUT", 5*time.Second),
		SessionCookieName: env("SESSION_COOKIE_NAME", "ournezt_session"),
		SessionCookieMax:  envDuration("SESSION_COOKIE_MAX_AGE", 24*time.Hour),
		CookieSecure:      envBool("SESSION_COOKIE_SECURE", false),
	}
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
