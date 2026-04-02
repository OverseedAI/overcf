package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/dns"
	"github.com/OverseedAI/overcf/internal/types"
)

func listAllRecords(ctx context.Context, cf *cloudflare.Client, zoneID string) ([]dns.RecordResponse, error) {
	var allRecords []dns.RecordResponse
	iter := cf.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
	})
	for iter.Next() {
		allRecords = append(allRecords, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return allRecords, nil
}

func dnsRecordFromResponse(r dns.RecordResponse) (types.DNSRecord, error) {
	record := types.DNSRecord{
		ID:      r.ID,
		Type:    types.RecordType(r.Type),
		Name:    r.Name,
		Content: r.Content,
		TTL:     int64(r.TTL),
		Proxied: r.Proxied,
	}

	if record.Type == types.RecordTypeMX || record.Type == types.RecordTypeSRV {
		priority := int64(r.Priority)
		record.Priority = &priority
	} else if r.Priority != 0 {
		priority := int64(r.Priority)
		record.Priority = &priority
	}

	switch strings.ToUpper(string(r.Type)) {
	case "SRV":
		if err := fillSRVData(&record, r.Data); err != nil {
			return types.DNSRecord{}, err
		}
	case "CAA":
		if err := fillCAAData(&record, r.Data); err != nil {
			return types.DNSRecord{}, err
		}
	}

	return record, nil
}

func recordIdentityKeyFromResponse(r dns.RecordResponse) (string, error) {
	record, err := dnsRecordFromResponse(r)
	if err != nil {
		return "", err
	}
	return recordIdentityKeyFromRecord(record)
}

func recordIdentityKeyFromRecord(record types.DNSRecord) (string, error) {
	recordType := strings.ToUpper(string(record.Type))
	switch record.Type {
	case types.RecordTypeMX:
		if record.Priority == nil {
			return "", fmt.Errorf("MX records require priority for identity matching")
		}
		return fmt.Sprintf("%s|%s|%s|%d", recordType, record.Name, record.Content, *record.Priority), nil
	case types.RecordTypeSRV:
		if record.Priority == nil || record.Port == nil || record.Weight == nil || record.Target == "" {
			return "", fmt.Errorf("SRV records require priority, port, weight, and target for identity matching")
		}
		return fmt.Sprintf("%s|%s|%d|%d|%d|%s", recordType, record.Name, *record.Priority, *record.Port, *record.Weight, record.Target), nil
	case types.RecordTypeCAA:
		if record.Flags == nil || record.Tag == "" || record.Value == "" {
			return "", fmt.Errorf("CAA records require flags, tag, and value for identity matching")
		}
		return fmt.Sprintf("%s|%s|%d|%s|%s", recordType, record.Name, *record.Flags, record.Tag, record.Value), nil
	default:
		return fmt.Sprintf("%s|%s|%s", recordType, record.Name, record.Content), nil
	}
}

func recordsEquivalent(desired types.DNSRecord, existing dns.RecordResponse) (bool, error) {
	current, err := dnsRecordFromResponse(existing)
	if err != nil {
		return false, err
	}

	if desired.Type != current.Type || desired.Name != current.Name {
		return false, nil
	}

	if desired.Proxied != current.Proxied {
		return false, nil
	}

	if desired.TTL != 0 && desired.TTL != current.TTL {
		return false, nil
	}

	switch desired.Type {
	case types.RecordTypeMX:
		if desired.Content != current.Content {
			return false, nil
		}
		return intPtrEqual(desired.Priority, current.Priority), nil
	case types.RecordTypeSRV:
		if !intPtrEqual(desired.Priority, current.Priority) {
			return false, nil
		}
		if !intPtrEqual(desired.Port, current.Port) {
			return false, nil
		}
		if !intPtrEqual(desired.Weight, current.Weight) {
			return false, nil
		}
		return desired.Target == current.Target, nil
	case types.RecordTypeCAA:
		if !intPtrEqual(desired.Flags, current.Flags) {
			return false, nil
		}
		if desired.Tag != current.Tag {
			return false, nil
		}
		return desired.Value == current.Value, nil
	default:
		return desired.Content == current.Content, nil
	}
}

func intPtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func fillSRVData(record *types.DNSRecord, data any) error {
	switch v := data.(type) {
	case dns.SRVRecordData:
		port := int64(v.Port)
		weight := int64(v.Weight)
		record.Port = &port
		record.Weight = &weight
		record.Target = v.Target
		if record.Priority == nil && v.Priority != 0 {
			priority := int64(v.Priority)
			record.Priority = &priority
		}
	case map[string]any:
		port, ok := floatFromAny(v["port"])
		if !ok {
			return fmt.Errorf("missing SRV port data")
		}
		weight, ok := floatFromAny(v["weight"])
		if !ok {
			return fmt.Errorf("missing SRV weight data")
		}
		target, ok := stringFromAny(v["target"])
		if !ok {
			return fmt.Errorf("missing SRV target data")
		}
		priority, ok := floatFromAny(v["priority"])
		if ok && record.Priority == nil {
			p := int64(priority)
			record.Priority = &p
		}
		portVal := int64(port)
		weightVal := int64(weight)
		record.Port = &portVal
		record.Weight = &weightVal
		record.Target = target
	case nil:
		return fmt.Errorf("missing SRV data")
	default:
		return fmt.Errorf("unsupported SRV data type: %T", data)
	}

	return nil
}

func fillCAAData(record *types.DNSRecord, data any) error {
	switch v := data.(type) {
	case dns.CAARecordData:
		flags := int64(v.Flags)
		record.Flags = &flags
		record.Tag = v.Tag
		record.Value = v.Value
	case map[string]any:
		flags, ok := floatFromAny(v["flags"])
		if !ok {
			return fmt.Errorf("missing CAA flags data")
		}
		tag, ok := stringFromAny(v["tag"])
		if !ok {
			return fmt.Errorf("missing CAA tag data")
		}
		value, ok := stringFromAny(v["value"])
		if !ok {
			return fmt.Errorf("missing CAA value data")
		}
		flagsVal := int64(flags)
		record.Flags = &flagsVal
		record.Tag = tag
		record.Value = value
	case nil:
		return fmt.Errorf("missing CAA data")
	default:
		return fmt.Errorf("unsupported CAA data type: %T", data)
	}

	return nil
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f, true
		}
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func stringFromAny(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}
