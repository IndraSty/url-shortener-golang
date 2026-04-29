package geoip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// geoIPKeyPrefix is the Redis key prefix for cached geo lookups.
	// Format: geo:{ip}
	geoIPKeyPrefix = "geo:"

	// geoIPTTL — cache geo results for 24 hours.
	// IP locations don't change often; this keeps us well under ip-api rate limits.
	geoIPTTL = 24 * time.Hour
)

// RedisGeoCache implements GeoCache using Redis.
type RedisGeoCache struct {
	client *redis.Client
}

// NewRedisGeoCache creates a Redis-backed geo cache.
func NewRedisGeoCache(client *redis.Client) GeoCache {
	return &RedisGeoCache{client: client}
}

func (c *RedisGeoCache) GetGeoIP(ctx context.Context, ip string) (*Location, error) {
	key := geoIPKeyPrefix + ip

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // cache miss
		}
		return nil, fmt.Errorf("geoCache.Get: %w", err)
	}

	var loc Location
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, nil // corrupted — treat as miss
	}

	return &loc, nil
}

func (c *RedisGeoCache) SetGeoIP(ctx context.Context, ip string, loc *Location) error {
	key := geoIPKeyPrefix + ip

	data, err := json.Marshal(loc)
	if err != nil {
		return fmt.Errorf("geoCache.Set marshal: %w", err)
	}

	if err := c.client.Set(ctx, key, data, geoIPTTL).Err(); err != nil {
		return fmt.Errorf("geoCache.Set: %w", err)
	}

	return nil
}
