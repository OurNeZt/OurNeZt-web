package web

import (
	"net/http"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
)

// NewHTTPServer bounds headers, complete request reads, response writes, and
// idle keep-alive connections. Body size limits are enforced by the router.
func NewHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.WebAddr,
		Handler:           handler,
		ReadHeaderTimeout: httpTimeout(cfg.ReadHeaderTimeout, config.DefaultReadHeaderTimeout),
		ReadTimeout:       httpTimeout(cfg.ReadTimeout, config.DefaultReadTimeout),
		WriteTimeout:      httpTimeout(cfg.WriteTimeout, config.DefaultWriteTimeout),
		IdleTimeout:       httpTimeout(cfg.IdleTimeout, config.DefaultIdleTimeout),
	}
}

func httpTimeout(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
