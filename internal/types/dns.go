// Package types defines shared types for DNS and zone operations.
package types

import (
	"fmt"
	"net"
	"strings"
)

// RecordType represents a DNS record type.
type RecordType string

// Supported DNS record types.
const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeMX    RecordType = "MX"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeNS    RecordType = "NS"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypeCAA   RecordType = "CAA"
	RecordTypePTR   RecordType = "PTR"
)

// AllRecordTypes returns all supported record types.
func AllRecordTypes() []RecordType {
	return []RecordType{
		RecordTypeA,
		RecordTypeAAAA,
		RecordTypeCNAME,
		RecordTypeMX,
		RecordTypeTXT,
		RecordTypeNS,
		RecordTypeSRV,
		RecordTypeCAA,
		RecordTypePTR,
	}
}

// IsValid checks if the record type is supported.
func (t RecordType) IsValid() bool {
	for _, valid := range AllRecordTypes() {
		if t == valid {
			return true
		}
	}
	return false
}

// RequiresPriority returns true if this record type requires a priority field.
func (t RecordType) RequiresPriority() bool {
	return t == RecordTypeMX || t == RecordTypeSRV
}

// SupportsProxy returns true if this record type can be proxied through Cloudflare.
func (t RecordType) SupportsProxy() bool {
	return t == RecordTypeA || t == RecordTypeAAAA || t == RecordTypeCNAME
}

// ValidateContent validates the content field for this record type.
func (t RecordType) ValidateContent(content string) error {
	switch t {
	case RecordTypeA:
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IPv4 address: %s", content)
		}
	case RecordTypeAAAA:
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid IPv6 address: %s", content)
		}
	case RecordTypeCNAME, RecordTypeMX, RecordTypeNS, RecordTypePTR:
		if content == "" {
			return fmt.Errorf("hostname cannot be empty")
		}
	case RecordTypeTXT:
		// TXT records can contain almost anything
		if len(content) > 2048 {
			return fmt.Errorf("TXT content exceeds maximum length of 2048 characters")
		}
	}
	return nil
}

// DNSRecord represents a DNS record for import/export operations.
type DNSRecord struct {
	// ID is the Cloudflare record ID (empty for new records).
	ID string `json:"id,omitempty" yaml:"id,omitempty"`

	// Type is the DNS record type (A, AAAA, CNAME, etc.).
	Type RecordType `json:"type" yaml:"type"`

	// Name is the fully qualified record name.
	Name string `json:"name" yaml:"name"`

	// Content is the record value (IP address, hostname, text, etc.).
	Content string `json:"content" yaml:"content"`

	// TTL is the time-to-live in seconds (1 = automatic).
	TTL int64 `json:"ttl,omitempty" yaml:"ttl,omitempty"`

	// Proxied indicates whether the record is proxied through Cloudflare.
	Proxied bool `json:"proxied,omitempty" yaml:"proxied,omitempty"`

	// Priority is used for MX and SRV records.
	Priority *int64 `json:"priority,omitempty" yaml:"priority,omitempty"`

	// SRV fields (required for SRV records).
	Port   *int64 `json:"port,omitempty" yaml:"port,omitempty"`
	Weight *int64 `json:"weight,omitempty" yaml:"weight,omitempty"`
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// CAA fields (required for CAA records).
	Flags *int64 `json:"flags,omitempty" yaml:"flags,omitempty"`
	Tag   string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// Validate checks if the DNS record is valid.
func (r *DNSRecord) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("invalid record type: %s", r.Type)
	}

	if r.Name == "" {
		return fmt.Errorf("record name is required")
	}

	switch r.Type {
	case RecordTypeSRV:
		if r.Priority == nil {
			return fmt.Errorf("%s records require a priority", r.Type)
		}
		if r.Port == nil {
			return fmt.Errorf("%s records require a port", r.Type)
		}
		if r.Weight == nil {
			return fmt.Errorf("%s records require a weight", r.Type)
		}
		if r.Target == "" {
			return fmt.Errorf("%s records require a target", r.Type)
		}
	case RecordTypeCAA:
		if r.Flags == nil {
			return fmt.Errorf("%s records require flags", r.Type)
		}
		if r.Tag == "" {
			return fmt.Errorf("%s records require a tag", r.Type)
		}
		if r.Value == "" {
			return fmt.Errorf("%s records require a value", r.Type)
		}
	default:
		if r.Content == "" {
			return fmt.Errorf("record content is required")
		}
	}

	if r.Content != "" {
		if err := r.Type.ValidateContent(r.Content); err != nil {
			return err
		}
	}

	if r.Type.RequiresPriority() && r.Priority == nil {
		return fmt.Errorf("%s records require a priority", r.Type)
	}

	if r.Priority != nil && *r.Priority < 0 {
		return fmt.Errorf("priority must be >= 0")
	}

	if r.Port != nil && *r.Port <= 0 {
		return fmt.Errorf("port must be > 0")
	}

	if r.Weight != nil && *r.Weight < 0 {
		return fmt.Errorf("weight must be >= 0")
	}

	if r.Flags != nil && *r.Flags < 0 {
		return fmt.Errorf("flags must be >= 0")
	}

	if r.Proxied && !r.Type.SupportsProxy() {
		return fmt.Errorf("%s records cannot be proxied", r.Type)
	}

	return nil
}

// ShortID returns a truncated ID for display purposes.
func (r *DNSRecord) ShortID() string {
	if len(r.ID) > 8 {
		return r.ID[:8]
	}
	return r.ID
}

// ProxiedString returns "yes" or "no" for display.
func (r *DNSRecord) ProxiedString() string {
	if r.Proxied {
		return "yes"
	}
	return "no"
}

// TTLString returns the TTL as a string, with "auto" for automatic.
func (r *DNSRecord) TTLString() string {
	if r.TTL == 1 {
		return "auto"
	}
	return fmt.Sprintf("%d", r.TTL)
}

// ParseRecordType parses a string into a RecordType.
func ParseRecordType(s string) (RecordType, error) {
	t := RecordType(strings.ToUpper(s))
	if !t.IsValid() {
		return "", fmt.Errorf("unsupported record type: %s", s)
	}
	return t, nil
}
