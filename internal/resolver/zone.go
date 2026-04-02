// Package resolver provides zone name to ID resolution.
package resolver

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/zones"
)

// zoneIDRegex matches Cloudflare zone IDs (32 hex characters).
var zoneIDRegex = regexp.MustCompile(`^[a-f0-9]{32}$`)

// ZoneResolver resolves domain names to zone IDs.
type ZoneResolver struct {
	client *cloudflare.Client
	cache  map[string]string
	mu     sync.RWMutex
}

// NewZoneResolver creates a new zone resolver.
func NewZoneResolver(client *cloudflare.Client) *ZoneResolver {
	return &ZoneResolver{
		client: client,
		cache:  make(map[string]string),
	}
}

// Resolve accepts either a zone ID or domain name and returns the zone ID.
// If the input looks like a zone ID (32 hex chars), it's returned directly.
// Otherwise, it's looked up as a domain name.
func (r *ZoneResolver) Resolve(ctx context.Context, input string) (string, error) {
	// If it looks like a zone ID, use directly
	if zoneIDRegex.MatchString(input) {
		return input, nil
	}

	// Check cache
	r.mu.RLock()
	if id, ok := r.cache[input]; ok {
		r.mu.RUnlock()
		return id, nil
	}
	r.mu.RUnlock()

	// Look up by domain name
	resp, err := r.client.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(input),
	})
	if err != nil {
		return "", fmt.Errorf("failed to look up zone: %w", err)
	}

	if len(resp.Result) == 0 {
		return "", fmt.Errorf("zone not found: %s", input)
	}

	id := resp.Result[0].ID

	// Cache the result
	r.mu.Lock()
	r.cache[input] = id
	r.mu.Unlock()

	return id, nil
}

// IsZoneID checks if the input looks like a zone ID.
func IsZoneID(input string) bool {
	return zoneIDRegex.MatchString(input)
}
