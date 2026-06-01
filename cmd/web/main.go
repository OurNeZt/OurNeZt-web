package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	"github.com/OurNeZt/ournezt-web/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))

	clients, err := core.NewClients(ctx, cfg)
	if err != nil {
		logger.Error("dial core grpc", "addr", cfg.CoreGRPCAddr, "tls", cfg.CoreGRPCUseTLS, "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := clients.Close(); closeErr != nil {
			logger.Error("close core grpc clients", "error", closeErr)
		}
	}()

	router, err := web.NewRouter(cfg, clients)
	if err != nil {
		logger.Error("build router", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.WebAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		webTLS := cfg.WebTLSCertFile != "" && cfg.WebTLSKeyFile != ""
		logger.Info("ournezt web started", "web_addr", cfg.WebAddr, "web_tls", webTLS, "core_grpc_addr", cfg.CoreGRPCAddr, "core_grpc_tls", cfg.CoreGRPCUseTLS)
		var serveErr error
		if webTLS {
			serveErr = httpServer.ListenAndServeTLS(cfg.WebTLSCertFile, cfg.WebTLSKeyFile)
		} else {
			serveErr = httpServer.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("http server stopped unexpectedly", "error", serveErr)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	logger.Info("ournezt web shut down")
}
