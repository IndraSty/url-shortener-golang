package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
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
	// Load and validate config — fail fast if required env vars are missing
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("FATAL: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Initialize structured logger
	log := logger.New(cfg.App.Env)

	log.Info().
		Str("env", cfg.App.Env).
		Str("port", cfg.App.Port).
		Str("base_url", cfg.App.BaseURL).
		Msg("configuration loaded successfully")

	// Root context — cancelled on shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// TODO: wire up DB, Redis, router in Stage 3+
	log.Info().Msg("server initialized — wiring continues in next stages")

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
	case <-ctx.Done():
	}

	// Allow 10s for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	log.Info().Msg("server stopped cleanly")
}
