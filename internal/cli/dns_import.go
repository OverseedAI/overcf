package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/dns"
	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/client"
	"github.com/OverseedAI/overcf/internal/config"
	"github.com/OverseedAI/overcf/internal/confirm"
	"github.com/OverseedAI/overcf/internal/exitcode"
	"github.com/OverseedAI/overcf/internal/resolver"
	"github.com/OverseedAI/overcf/internal/types"
)

var (
	dnsImportFile    string
	dnsImportStdin   bool
	dnsImportFormat  string
	dnsImportReplace bool
)

var dnsImportCmd = &cobra.Command{
	Use:   "import <zone>",
	Short: "Import DNS records",
	Long: `Import DNS records from JSON or CSV.

JSON accepts either a raw array of records or an overcf list response.
CSV accepts headers: id,type,name,content,ttl,proxied,priority,port,weight,target,flags,tag,value`,
	Example: `  overcf dns import example.com --file records.json
  overcf dns import example.com --file records.csv --input-format csv
  cat records.json | overcf dns import example.com --stdin`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "AUTH_REQUIRED", err.Error(), nil)
			os.Exit(exitcode.AuthError)
			return nil
		}

		cf, err := client.Get(cfg)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "AUTH_ERROR", err.Error(), nil)
			os.Exit(exitcode.AuthError)
			return nil
		}

		ctx := context.Background()
		res := resolver.NewZoneResolver(cf)

		zoneID, err := res.Resolve(ctx, args[0])
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "ZONE_NOT_FOUND", err.Error(), nil)
			os.Exit(exitcode.NotFound)
			return nil
		}

		format, err := detectImportFormat()
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		payload, err := readImportPayload()
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		records, err := parseImportRecords(payload, format)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}
		if len(records) == 0 {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "No records found in import file", nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		existing, err := listAllRecords(ctx, cf, zoneID)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		existingByID := make(map[string]dns.RecordResponse)
		existingByKey := make(map[string][]dns.RecordResponse)
		for _, r := range existing {
			existingByID[r.ID] = r
			key, err := recordIdentityKeyFromResponse(r)
			if err != nil {
				continue
			}
			existingByKey[key] = append(existingByKey[key], r)
		}

		created := 0
		updated := 0
		skipped := 0
		var errorsList []string
		keepIDs := make(map[string]struct{})

		for i, record := range records {
			if err := record.Validate(); err != nil {
				errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
				continue
			}

			if record.ID != "" {
				existingRecord, ok := existingByID[record.ID]
				if !ok {
					errorsList = append(errorsList, fmt.Sprintf("record %d: record ID not found: %s", i+1, record.ID))
					continue
				}
				if record.Type != "" && strings.ToUpper(string(existingRecord.Type)) != strings.ToUpper(string(record.Type)) {
					errorsList = append(errorsList, fmt.Sprintf("record %d: type mismatch for ID %s", i+1, record.ID))
					continue
				}

				desired := record
				if desired.TTL == 0 {
					desired.TTL = int64(existingRecord.TTL)
				}

				equal, err := recordsEquivalent(desired, existingRecord)
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}
				if equal {
					skipped++
					keepIDs[existingRecord.ID] = struct{}{}
					continue
				}

				body, err := buildRecordEditBody(desired)
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}

				updatedRecord, err := cf.DNS.Records.Edit(ctx, record.ID, dns.RecordEditParams{
					ZoneID: cloudflare.F(zoneID),
					Body:   body,
				})
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}
				updated++
				keepIDs[updatedRecord.ID] = struct{}{}
				continue
			}

			key, err := recordIdentityKeyFromRecord(record)
			if err != nil {
				errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
				continue
			}

			if matches := existingByKey[key]; len(matches) > 0 {
				existingRecord := matches[0]
				desired := record
				if desired.TTL == 0 {
					desired.TTL = int64(existingRecord.TTL)
				}

				equal, err := recordsEquivalent(desired, existingRecord)
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}
				if equal {
					skipped++
					keepIDs[existingRecord.ID] = struct{}{}
					continue
				}

				body, err := buildRecordEditBody(desired)
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}

				updatedRecord, err := cf.DNS.Records.Edit(ctx, existingRecord.ID, dns.RecordEditParams{
					ZoneID: cloudflare.F(zoneID),
					Body:   body,
				})
				if err != nil {
					errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
					continue
				}
				updated++
				keepIDs[updatedRecord.ID] = struct{}{}
				continue
			}

			body, err := buildRecordNewBody(record)
			if err != nil {
				errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
				continue
			}

			createdRecord, err := cf.DNS.Records.New(ctx, dns.RecordNewParams{
				ZoneID: cloudflare.F(zoneID),
				Body:   body,
			})
			if err != nil {
				errorsList = append(errorsList, fmt.Sprintf("record %d: %v", i+1, err))
				continue
			}
			created++
			keepIDs[createdRecord.ID] = struct{}{}
		}

		deleted := 0
		if dnsImportReplace && len(errorsList) == 0 {
			var deleteTargets []string
			var deleteRecords []dns.RecordResponse
			for _, r := range existing {
				if _, ok := keepIDs[r.ID]; ok {
					continue
				}
				deleteRecords = append(deleteRecords, r)
				deleteTargets = append(deleteTargets, recordSummary(r))
			}

			if len(deleteRecords) > 0 {
				if !confirm.DestructiveMultiple("delete", deleteTargets, flagYes) {
					fmt.Println("Cancelled.")
					return nil
				}

				for _, r := range deleteRecords {
					if _, err := cf.DNS.Records.Delete(ctx, r.ID, dns.RecordDeleteParams{
						ZoneID: cloudflare.F(zoneID),
					}); err != nil {
						errorsList = append(errorsList, fmt.Sprintf("delete %s: %v", r.ID, err))
						continue
					}
					deleted++
				}
			}
		}

		formatter := getFormatter()

		if len(errorsList) > 0 {
			details := map[string]any{
				"created": created,
				"updated": updated,
				"skipped": skipped,
				"deleted": deleted,
				"errors":  errorsList,
			}
			formatter.FormatError(os.Stderr, "IMPORT_FAILED", "One or more records failed to import", details)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		result := importResult{
			Created: created,
			Updated: updated,
			Skipped: skipped,
			Deleted: deleted,
		}

		if isJSONOutput() {
			return formatter.Format(os.Stdout, result)
		}

		fmt.Printf("Import complete: created %d, updated %d, skipped %d", created, updated, skipped)
		if deleted > 0 {
			fmt.Printf(", deleted %d", deleted)
		}
		fmt.Println()

		return nil
	},
}

type importResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Deleted int `json:"deleted,omitempty"`
}

func init() {
	dnsImportCmd.Flags().StringVar(&dnsImportFile, "file", "", "Import file path")
	dnsImportCmd.Flags().BoolVar(&dnsImportStdin, "stdin", false, "Read records from stdin")
	dnsImportCmd.Flags().StringVar(&dnsImportFormat, "input-format", "", "Input format: json or csv (auto-detect from file)")
	dnsImportCmd.Flags().BoolVar(&dnsImportReplace, "replace", false, "Delete records not present in the import file")

	dnsCmd.AddCommand(dnsImportCmd)
}

func detectImportFormat() (string, error) {
	format := strings.ToLower(strings.TrimSpace(dnsImportFormat))
	if format == "" && dnsImportFile != "" {
		ext := strings.ToLower(filepath.Ext(dnsImportFile))
		switch ext {
		case ".json":
			format = "json"
		case ".csv":
			format = "csv"
		}
	}
	if format == "" {
		format = "json"
	}

	switch format {
	case "json", "csv":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported input format: %s", format)
	}
}

func readImportPayload() ([]byte, error) {
	if dnsImportStdin {
		return io.ReadAll(os.Stdin)
	}
	if dnsImportFile == "" {
		return nil, fmt.Errorf("provide --file or --stdin for import")
	}
	return os.ReadFile(dnsImportFile)
}

func parseImportRecords(payload []byte, format string) ([]types.DNSRecord, error) {
	switch format {
	case "json":
		return parseImportJSON(payload)
	case "csv":
		return parseImportCSV(payload)
	default:
		return nil, fmt.Errorf("unsupported input format: %s", format)
	}
}

func parseImportJSON(payload []byte) ([]types.DNSRecord, error) {
	var records []types.DNSRecord
	if err := json.Unmarshal(payload, &records); err == nil {
		return records, nil
	}

	var wrapper struct {
		Data    []types.DNSRecord `json:"data"`
		Records []types.DNSRecord `json:"records"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid JSON import format")
	}
	if len(wrapper.Data) > 0 {
		return wrapper.Data, nil
	}
	if len(wrapper.Records) > 0 {
		return wrapper.Records, nil
	}
	return nil, fmt.Errorf("invalid JSON import format")
}

func parseImportCSV(payload []byte) ([]types.DNSRecord, error) {
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV has no rows")
	}

	header := rows[0]
	colIndex := defaultCSVColumns()
	start := 0
	if looksLikeCSVHeader(header) {
		colIndex = buildCSVColumnIndex(header)
		start = 1
	}

	var records []types.DNSRecord
	for i := start; i < len(rows); i++ {
		row := rows[i]
		if isEmptyRow(row) {
			continue
		}
		record, err := parseCSVRow(row, colIndex)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		records = append(records, record)
	}

	return records, nil
}

func defaultCSVColumns() map[string]int {
	return map[string]int{
		"id":       0,
		"type":     1,
		"name":     2,
		"content":  3,
		"ttl":      4,
		"proxied":  5,
		"priority": 6,
		"port":     7,
		"weight":   8,
		"target":   9,
		"flags":    10,
		"tag":      11,
		"value":    12,
	}
}

func looksLikeCSVHeader(row []string) bool {
	for _, value := range row {
		if strings.EqualFold(strings.TrimSpace(value), "type") {
			return true
		}
	}
	return false
}

func buildCSVColumnIndex(header []string) map[string]int {
	index := make(map[string]int)
	for i, value := range header {
		key := strings.ToLower(strings.TrimSpace(value))
		if key != "" {
			index[key] = i
		}
	}
	return index
}

func parseCSVRow(row []string, colIndex map[string]int) (types.DNSRecord, error) {
	get := func(key string) string {
		idx, ok := colIndex[key]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	recordType := get("type")
	if recordType == "" {
		return types.DNSRecord{}, fmt.Errorf("missing type")
	}
	parsedType, err := types.ParseRecordType(recordType)
	if err != nil {
		return types.DNSRecord{}, err
	}

	record := types.DNSRecord{
		ID:      get("id"),
		Type:    parsedType,
		Name:    get("name"),
		Content: get("content"),
	}

	if ttl := get("ttl"); ttl != "" {
		val, err := strconv.ParseInt(ttl, 10, 64)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid ttl: %s", ttl)
		}
		record.TTL = val
	}

	if proxied := get("proxied"); proxied != "" {
		val, err := parseBoolField(proxied)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid proxied value: %s", proxied)
		}
		record.Proxied = val
	}

	if priority := get("priority"); priority != "" {
		val, err := strconv.ParseInt(priority, 10, 64)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid priority: %s", priority)
		}
		record.Priority = &val
	}

	if port := get("port"); port != "" {
		val, err := strconv.ParseInt(port, 10, 64)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid port: %s", port)
		}
		record.Port = &val
	}

	if weight := get("weight"); weight != "" {
		val, err := strconv.ParseInt(weight, 10, 64)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid weight: %s", weight)
		}
		record.Weight = &val
	}

	record.Target = get("target")

	if flags := get("flags"); flags != "" {
		val, err := strconv.ParseInt(flags, 10, 64)
		if err != nil {
			return types.DNSRecord{}, fmt.Errorf("invalid flags: %s", flags)
		}
		record.Flags = &val
	}

	record.Tag = get("tag")
	record.Value = get("value")

	return record, nil
}

func parseBoolField(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "t", "yes", "y", "1":
		return true, nil
	case "false", "f", "no", "n", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

func isEmptyRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func recordSummary(record dns.RecordResponse) string {
	content := record.Content
	switch strings.ToUpper(string(record.Type)) {
	case "SRV":
		if data, err := dnsRecordFromResponse(record); err == nil {
			if data.Priority != nil && data.Port != nil && data.Weight != nil && data.Target != "" {
				content = fmt.Sprintf("%d %d %d %s", *data.Priority, *data.Weight, *data.Port, data.Target)
			}
		}
	case "CAA":
		if data, err := dnsRecordFromResponse(record); err == nil {
			if data.Flags != nil && data.Tag != "" && data.Value != "" {
				content = fmt.Sprintf("%d %s %s", *data.Flags, data.Tag, data.Value)
			}
		}
	}
	return fmt.Sprintf("%s %s -> %s", record.Type, record.Name, content)
}
