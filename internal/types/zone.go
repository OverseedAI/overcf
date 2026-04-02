package types

// Zone represents a Cloudflare DNS zone.
type Zone struct {
	// ID is the unique zone identifier.
	ID string `json:"id" yaml:"id"`

	// Name is the domain name.
	Name string `json:"name" yaml:"name"`

	// Status is the zone status (active, pending, moved, etc.).
	Status string `json:"status" yaml:"status"`

	// Plan is the Cloudflare plan name.
	Plan string `json:"plan,omitempty" yaml:"plan,omitempty"`

	// NameServers are the assigned Cloudflare nameservers.
	NameServers []string `json:"name_servers,omitempty" yaml:"name_servers,omitempty"`
}

// ShortID returns a truncated ID for display purposes.
func (z *Zone) ShortID() string {
	if len(z.ID) > 12 {
		return z.ID[:12] + "..."
	}
	return z.ID
}
