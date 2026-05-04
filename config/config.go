package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	QStash   QStashConfig
	Grafana  GrafanaConfig
}

type AppConfig struct {
	Env         string
	Port        string
	BaseURL     string
	CORSOrigins []string
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

type QStashConfig struct {
	Token      string
	CurrentKey string
	NextKey    string
	URL        string
}

// this is for the remote write metrics to Grafana Cloud. We keep it in the main config for simplicity,
// you can delete it if you don't need the Grafana integration.
type GrafanaConfig struct {
	RemoteWriteURL string
	Username       string
	APIKey         string
}

// Load reads config from .env file and environment variables.
// Environment variables always take precedence over .env file values.
func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read .env if it exists; production relies on real env vars
	if err := viper.ReadInConfig(); err != nil {
		// Not fatal — production won't have a .env file
		_ = err
	}

	setDefaults()

	cfg := &Config{
		App: AppConfig{
			Env:         viper.GetString("APP_ENV"),
			Port:        viper.GetString("APP_PORT"),
			BaseURL:     viper.GetString("BASE_URL"),
			CORSOrigins: parseCORSOrigins(viper.GetString("CORS_ALLOWED_ORIGINS")),
		},
		Database: DatabaseConfig{
			URL:             viper.GetString("DATABASE_URL"),
			MaxConns:        viper.GetInt32("DB_MAX_CONNS"),
			MinConns:        viper.GetInt32("DB_MIN_CONNS"),
			MaxConnLifetime: viper.GetDuration("DB_MAX_CONN_LIFETIME"),
			MaxConnIdleTime: viper.GetDuration("DB_MAX_CONN_IDLE_TIME"),
		},
		Redis: RedisConfig{
			URL:      viper.GetString("REDIS_URL"),
			Password: viper.GetString("REDIS_PASSWORD"),
			DB:       viper.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			AccessSecret:  viper.GetString("JWT_ACCESS_SECRET"),
			RefreshSecret: viper.GetString("JWT_REFRESH_SECRET"),
			AccessExpiry:  viper.GetDuration("JWT_ACCESS_EXPIRY"),
			RefreshExpiry: viper.GetDuration("JWT_REFRESH_EXPIRY"),
		},
		QStash: QStashConfig{
			Token:      viper.GetString("QSTASH_TOKEN"),
			CurrentKey: viper.GetString("QSTASH_CURRENT_SIGNING_KEY"),
			NextKey:    viper.GetString("QSTASH_NEXT_SIGNING_KEY"),
			URL:        viper.GetString("QSTASH_URL"),
		},
		Grafana: GrafanaConfig{
			RemoteWriteURL: viper.GetString("GRAFANA_REMOTE_WRITE_URL"),
			Username:       viper.GetString("GRAFANA_USERNAME"),
			APIKey:         viper.GetString("GRAFANA_API_KEY"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validate ensures all required secrets are present.
// Fail fast at startup — better than a cryptic runtime error later.
func (c *Config) validate() error {
	var errs []string

	if c.Database.URL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.Redis.URL == "" {
		errs = append(errs, "REDIS_URL is required")
	}
	if c.JWT.AccessSecret == "" {
		errs = append(errs, "JWT_ACCESS_SECRET is required")
	}
	if c.JWT.RefreshSecret == "" {
		errs = append(errs, "JWT_REFRESH_SECRET is required")
	}
	if c.App.BaseURL == "" {
		errs = append(errs, "BASE_URL is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// IsProd returns true when running in production environment.
func (c *Config) IsProd() bool {
	return c.App.Env == "production"
}

func setDefaults() {
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("APP_PORT", "8080")
	viper.SetDefault("BASE_URL", "http://localhost:8080")

	viper.SetDefault("DB_MAX_CONNS", 10)
	viper.SetDefault("DB_MIN_CONNS", 2)
	viper.SetDefault("DB_MAX_CONN_LIFETIME", "1h")
	viper.SetDefault("DB_MAX_CONN_IDLE_TIME", "30m")

	viper.SetDefault("REDIS_DB", 0)

	viper.SetDefault("JWT_ACCESS_EXPIRY", "15m")
	viper.SetDefault("JWT_REFRESH_EXPIRY", "168h")
}

func parseCORSOrigins(raw string) []string {
	if raw == "" {
		return []string{"http://localhost:3000"}
	}

	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}
