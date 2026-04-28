package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/redis/go-redis/v9"
)

// NewClient creates an Upstash Redis client.
// Upstash uses TLS (rediss://) — the go-redis client handles this automatically
// when the URL scheme is rediss://.
func NewClient(cfg config.RedisConfig) (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	// Override password if explicitly set (some Upstash URLs embed it,
	// but we support explicit override for flexibility)
	if cfg.Password != "" {
		opts.Password = cfg.Password
	}

	opts.DB = cfg.DB

	// Upstash free tier connection limits — keep pool small
	opts.PoolSize = 10
	opts.MinIdleConns = 2
	opts.ConnMaxIdleTime = 5 * time.Minute

	client := redis.NewClient(opts)

	// Verify connection on startup
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}
