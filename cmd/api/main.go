package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/IndraSty/url-shortener-golang/internal/repository/postgres"
	"github.com/IndraSty/url-shortener-golang/internal/repository/redis"
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
	// -------------------------------------------------------------------------
	// Config
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("FATAL: " + err.Error() + "\n")
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Logger
	// -------------------------------------------------------------------------
	log := logger.New(cfg.App.Env)
	log.Info().
		Str("env", cfg.App.Env).
		Str("port", cfg.App.Port).
		Msg("configuration loaded")

	// -------------------------------------------------------------------------
	// Database migrations — run on every startup, idempotent
	// -------------------------------------------------------------------------
	log.Info().Msg("running database migrations...")
	if err := postgres.RunMigrations(cfg.Database.URL, "./migrations"); err != nil {
		log.Fatal().Err(err).Msg("database migration failed")
	}
	log.Info().Msg("migrations applied successfully")

	// -------------------------------------------------------------------------
	// PostgreSQL connection pool
	// -------------------------------------------------------------------------
	ctx := context.Background()

	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer func() {
		dbPool.Close()
		log.Info().Msg("postgres connection pool closed")
	}()

	log.Info().
		Int32("max_conns", cfg.Database.MaxConns).
		Msg("postgres connected")

	// -------------------------------------------------------------------------
	// Redis client
	// -------------------------------------------------------------------------
	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Error().Err(err).Msg("redis close error")
		}
		log.Info().Msg("redis connection closed")
	}()

	log.Info().Msg("redis connected")

	// -------------------------------------------------------------------------
	// Graceful shutdown
	// -------------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// TODO: wire up Echo server in Stage 7+
	log.Info().Str("port", cfg.App.Port).Msg("ready — HTTP server wiring in next stages")

	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	log.Info().Msg("server stopped cleanly")
}
