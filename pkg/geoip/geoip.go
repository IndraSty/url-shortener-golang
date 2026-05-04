package geoip

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// apiURL is the ip-api.com endpoint. Free tier, no API key needed.
	// Fields param limits response to only what we need — smaller payload, faster parse.
	apiURL = "http://ip-api.com/json/%s?fields=status,country,countryCode,city,query"

	// requestTimeout is the max time we wait for a geo lookup.
	// If ip-api.com is slow, we return empty strings and continue — never block redirect.
	requestTimeout = 2 * time.Second

	// ip-api.com free tier limit: 45 requests/minute.
	// We cache results in Redis to stay well under this limit.
)

// Location holds the geo data resolved for an IP address.
type Location struct {
	Country     string // full country name e.g. "Indonesia"
	CountryCode string // ISO 3166-1 alpha-2 e.g. "ID"
	City        string
	IP          string // the queried IP (as returned by ip-api)
}

// apiResponse mirrors the ip-api.com JSON response.
type apiResponse struct {
	Status      string `json:"status"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
	Query       string `json:"query"`
}

// Client is a geo-IP resolver backed by ip-api.com.
type Client struct {
	http  *http.Client
	cache GeoCache // Redis-backed cache injected from outside
}

// GeoCache is the cache interface used by the GeoIP client.
// Keeping it as an interface allows testing without Redis.
type GeoCache interface {
	GetGeoIP(ctx context.Context, ip string) (*Location, error)
	SetGeoIP(ctx context.Context, ip string, loc *Location) error
}

// NewClient creates a GeoIP client with a cache backend.
func NewClient(cache GeoCache) *Client {
	return &Client{
		http: &http.Client{
			Timeout: requestTimeout,
		},
		cache: cache,
	}
}

// Lookup resolves an IP address to a Location.
// Cache is checked first — ip-api.com is only called on cache miss.
// Returns an empty Location (not an error) if geo resolution fails,
// so that the redirect path is never blocked by a geo lookup failure.
func (c *Client) Lookup(ctx context.Context, ip string) *Location {
	// Skip private/loopback IPs — ip-api can't resolve them
	if isPrivateIP(ip) {
		return &Location{IP: ip}
	}

	// Cache check first
	if loc, err := c.cache.GetGeoIP(ctx, ip); err == nil && loc != nil {
		return loc
	}

	// Call ip-api.com
	loc, err := c.fetchFromAPI(ctx, ip)
	if err != nil {
		// Geo failure is non-fatal — return empty location
		return &Location{IP: ip}
	}

	// Cache the result — fire and forget, don't block on cache write
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		c.cache.SetGeoIP(cacheCtx, ip, loc) //nolint:errcheck
	}()

	return loc
}

func (c *Client) fetchFromAPI(ctx context.Context, ip string) (*Location, error) {
	url := fmt.Sprintf(apiURL, ip)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("geoip: create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("geoip: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geoip: unexpected status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("geoip: decode response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("geoip: api returned status %q for ip %s", apiResp.Status, ip)
	}

	return &Location{
		Country:     apiResp.Country,
		CountryCode: strings.ToUpper(apiResp.CountryCode),
		City:        apiResp.City,
		IP:          apiResp.Query,
	}, nil
}

// MaskIP returns a privacy-safe version of an IP address.
// IPv4: replaces last two octets with 0 → "192.168.0.0"
// IPv6: keeps first 4 groups only → "2001:db8::"
// This is done at the application layer before any storage — never store full IPs.
func MaskIP(rawIP string) string {
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return ""
	}

	if ip.To4() != nil {
		// IPv4 — zero out last two octets
		parts := strings.Split(rawIP, ".")
		if len(parts) != 4 {
			return ""
		}
		return fmt.Sprintf("%s.%s.0.0", parts[0], parts[1])
	}

	// IPv6 — keep first 4 groups
	parts := strings.Split(ip.String(), ":")
	if len(parts) < 4 {
		return ip.String()
	}
	return fmt.Sprintf("%s:%s:%s:%s::", parts[0], parts[1], parts[2], parts[3])
}

// isPrivateIP returns true for loopback, private, and link-local addresses.
// These cannot be resolved by ip-api.com.
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}

	privateRanges := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
