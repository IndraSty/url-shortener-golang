package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IndraSty/url-shortener/config"
	"github.com/IndraSty/url-shortener/pkg/logger"
)

// @title           URL Shortener API
// @version         1.0
// @description     Enterprise URL Shortener with Analytics, A/B Testing, and Geo-targeting
// @termsOfService  http://swagger.io/terms/

// @contact.name   IndraSty
// @contact.url    https://github.com/IndraSty

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		// Can't use logger yet, fall back to stderr
		os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.App.Env)
	log.Info().Str("env", cfg.App.Env).Str("port", cfg.App.Port).Msg("starting url-shortener")

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// TODO: wire up server in Stage 2+
	// For now, block until signal received
	log.Info().Msg("server ready (wiring up in next stages)")

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutting down gracefully")
		cancel()
	case <-ctx.Done():
	}

	// Give in-flight requests 10s to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	log.Info().Msg("server stopped")
}
