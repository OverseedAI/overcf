// Package config handles configuration loading from environment variables.
package config

import (
	"errors"
	"os"
)

// ErrNoToken is returned when no API token is configured.
var ErrNoToken = errors.New("CLOUDFLARE_API_TOKEN environment variable is not set")

// Config holds the application configuration.
type Config struct {
	// APIToken is the Cloudflare API token for authentication.
	APIToken string

	// BaseURL overrides the default API base URL (useful for testing).
	BaseURL string

	// Debug enables debug logging when true.
	Debug bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if token == "" {
		return nil, ErrNoToken
	}

	return &Config{
		APIToken: token,
		BaseURL:  os.Getenv("CLOUDFLARE_API_URL"),
		Debug:    os.Getenv("OVERCF_DEBUG") == "1",
	}, nil
}

// MustLoad loads configuration and panics on error.
// Use this only in contexts where missing config is a fatal error.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}
