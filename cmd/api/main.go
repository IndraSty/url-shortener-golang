package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	deliveryhttp "github.com/IndraSty/url-shortener-golang/internal/delivery/http"
	"github.com/IndraSty/url-shortener-golang/internal/repository/postgres"
	redisrepo "github.com/IndraSty/url-shortener-golang/internal/repository/redis"
	"github.com/IndraSty/url-shortener-golang/internal/usecase"
	"github.com/IndraSty/url-shortener-golang/internal/worker"
	"github.com/IndraSty/url-shortener-golang/pkg/geoip"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/IndraSty/url-shortener-golang/pkg/metrics"
	"github.com/IndraSty/url-shortener-golang/pkg/qrcode"
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
	// =========================================================================
	// 1. Config
	// =========================================================================
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("FATAL: " + err.Error() + "\n")
		os.Exit(1)
	}

	// =========================================================================
	// 2. Logger
	// =========================================================================
	log := logger.New(cfg.App.Env)
	log.Info().
		Str("env", cfg.App.Env).
		Str("port", cfg.App.Port).
		Str("base_url", cfg.App.BaseURL).
		Msg("starting url-shortener")

	// =========================================================================
	// 3. Database migrations
	// =========================================================================
	log.Info().Msg("running database migrations...")
	if err := postgres.RunMigrations(cfg.Database.URL, "./migrations"); err != nil {
		log.Fatal().Err(err).Msg("migration failed")
	}
	log.Info().Msg("migrations up to date")

	// =========================================================================
	// 4. PostgreSQL connection pool
	// =========================================================================
	ctx := context.Background()

	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("postgres connection failed")
	}
	defer func() {
		dbPool.Close()
		log.Info().Msg("postgres pool closed")
	}()
	log.Info().Int32("max_conns", cfg.Database.MaxConns).Msg("postgres connected")

	// =========================================================================
	// 5. Redis client
	// =========================================================================
	redisClient, err := redisrepo.NewClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("redis connection failed")
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Error().Err(err).Msg("redis close error")
		}
		log.Info().Msg("redis closed")
	}()
	log.Info().Msg("redis connected")

	// =========================================================================
	// 6. Repositories
	// =========================================================================
	userRepo := postgres.NewUserRepository(dbPool)
	linkRepo := postgres.NewLinkRepository(dbPool)
	abTestRepo := postgres.NewABTestRepository(dbPool)
	geoRuleRepo := postgres.NewGeoRuleRepository(dbPool)
	analyticsRepo := postgres.NewAnalyticsRepository(dbPool)
	cacheRepo := redisrepo.NewCacheRepository(redisClient)

	// =========================================================================
	// 7. pkg layer
	// =========================================================================
	geoCache := geoip.NewRedisGeoCache(redisClient)
	geoClient := geoip.NewClient(geoCache)
	qrGen := qrcode.NewGenerator()

	// =========================================================================
	// 8. Usecases
	// =========================================================================
	authUsecase := usecase.NewAuthUsecase(userRepo, cfg.JWT)

	linkUsecase := usecase.NewLinkUsecase(
		linkRepo,
		abTestRepo,
		geoRuleRepo,
		cacheRepo,
		qrGen,
		cfg,
	)

	redirectUsecase := usecase.NewRedirectUsecase(
		linkRepo,
		cacheRepo,
		geoClient,
		cfg,
	)

	analyticsUsecase := usecase.NewAnalyticsUsecase(
		analyticsRepo,
		linkRepo,
		geoClient,
	)

	// =========================================================================
	// 9. Analytics worker (local dev fallback — production uses QStash push)
	// =========================================================================
	analyticsWorker := worker.NewAnalyticsWorker(
		analyticsUsecase,
		log,
		10000, // buffer: 10k events
		4,     // 4 concurrent DB writers
	)

	// =========================================================================
	// 10. HTTP Router
	// =========================================================================
	router := deliveryhttp.NewRouter(deliveryhttp.RouterDeps{
		Config:           cfg,
		AuthUsecase:      authUsecase,
		LinkUsecase:      linkUsecase,
		RedirectUsecase:  redirectUsecase,
		AnalyticsUsecase: analyticsUsecase,
		CacheRepo:        cacheRepo,
		Log:              log,
		DB:               dbPool,
		RedisClient:      redisClient,
	})

	// =========================================================================
	// 11. HTTP Server
	// =========================================================================
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.App.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// =========================================================================
	// 12. Start everything concurrently
	// =========================================================================
	workerCtx, workerCancel := context.WithCancel(context.Background())

	// Start analytics worker (no-op in prod when QStash is configured)
	if cfg.QStash.Token == "" {
		log.Info().Msg("QStash not configured — using in-process analytics worker")
		go analyticsWorker.Start(workerCtx)
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					metrics.AnalyticsQueueDepth.Set(
						float64(analyticsWorker.QueueDepth()),
					)
				case <-workerCtx.Done():
					return
				}
			}
		}()
	} else {
		log.Info().Msg("QStash configured — analytics worker disabled (using push mode)")
		workerCancel() // cancel immediately, worker not needed
	}

	// Start Grafana remote write
	go metrics.StartRemoteWrite(
		workerCtx,
		cfg.Grafana.RemoteWriteURL,
		cfg.Grafana.Username,
		cfg.Grafana.APIKey,
		log,
	)

	// Start HTTP server
	serverErr := make(chan error, 1)
	go func() {
		log.Info().Str("addr", server.Addr).Msg("HTTP server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// =========================================================================
	// 13. Graceful shutdown
	// =========================================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	case err := <-serverErr:
		log.Fatal().Err(err).Msg("server error")
	}

	log.Info().Msg("shutting down gracefully...")

	// Stop accepting new connections — give in-flight requests 10s
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server forced shutdown")
	}
	log.Info().Msg("HTTP server stopped")

	// Stop analytics worker — drain remaining events
	workerCancel()
	log.Info().Msg("shutdown complete")
}
