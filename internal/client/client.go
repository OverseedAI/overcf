// Package client provides a singleton Cloudflare API client.
package client

import (
	"sync"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/OverseedAI/overcf/internal/config"
)

var (
	instance *cloudflare.Client
	once     sync.Once
	initErr  error
)

// Get returns the singleton Cloudflare client.
// The client is initialized on first call with the provided configuration.
func Get(cfg *config.Config) (*cloudflare.Client, error) {
	once.Do(func() {
		opts := []option.RequestOption{
			option.WithAPIToken(cfg.APIToken),
		}

		if cfg.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(cfg.BaseURL))
		}

		instance = cloudflare.NewClient(opts...)
	})

	if initErr != nil {
		return nil, initErr
	}

	return instance, nil
}

// MustGet returns the singleton Cloudflare client, panicking on error.
func MustGet(cfg *config.Config) *cloudflare.Client {
	client, err := Get(cfg)
	if err != nil {
		panic(err)
	}
	return client
}
