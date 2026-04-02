package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/dns"
	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/client"
	"github.com/OverseedAI/overcf/internal/config"
	"github.com/OverseedAI/overcf/internal/confirm"
	"github.com/OverseedAI/overcf/internal/exitcode"
	"github.com/OverseedAI/overcf/internal/output"
	"github.com/OverseedAI/overcf/internal/resolver"
	"github.com/OverseedAI/overcf/internal/types"
)

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "Manage DNS records",
	Long:  "Commands for listing, creating, updating, and deleting DNS records.",
}

// dns list flags
var (
	dnsListType   string
	dnsListSearch string
)

var dnsListCmd = &cobra.Command{
	Use:   "list <zone>",
	Short: "List DNS records",
	Long:  "List all DNS records for a zone.",
	Example: `  overcf dns list example.com
  overcf dns list example.com --type A
  overcf dns list example.com --search www
  overcf dns list example.com --json`,
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

		params := dns.RecordListParams{
			ZoneID: cloudflare.F(zoneID),
		}
		if dnsListType != "" {
			params.Type = cloudflare.F(dns.RecordListParamsType(strings.ToUpper(dnsListType)))
		}
		if dnsListSearch != "" {
			params.Search = cloudflare.F(dnsListSearch)
		}

		// Use auto-paging to get all records
		var allRecords []dns.RecordResponse
		iter := cf.DNS.Records.ListAutoPaging(ctx, params)
		for iter.Next() {
			allRecords = append(allRecords, iter.Current())
		}
		if err := iter.Err(); err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			out := make([]types.DNSRecord, 0, len(allRecords))
			for _, r := range allRecords {
				record, err := dnsRecordFromResponse(r)
				if err != nil {
					formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
					os.Exit(exitcode.GeneralError)
					return nil
				}
				out = append(out, record)
			}
			return formatter.Format(os.Stdout, output.NewListSuccess(out))
		}

		headers := []string{"ID", "TYPE", "NAME", "CONTENT", "TTL", "PROXIED"}
		rows := make([][]string, 0, len(allRecords))
		for _, r := range allRecords {
			proxied := strconv.FormatBool(r.Proxied)
			if isTableOutput() {
				if r.Proxied {
					proxied = "yes"
				} else {
					proxied = "no"
				}
			}
			ttl := fmt.Sprintf("%v", r.TTL)
			id := r.ID
			content := r.Content
			if isTableOutput() {
				id = truncateID(r.ID)
				content = truncateContent(r.Content)
			}
			rows = append(rows, []string{
				id,
				string(r.Type),
				r.Name,
				content,
				ttl,
				proxied,
			})
		}

		return formatter.FormatList(os.Stdout, headers, rows)
	},
}

var dnsGetCmd = &cobra.Command{
	Use:   "get <zone> <record-id>",
	Short: "Get a DNS record",
	Long:  "Get details of a specific DNS record.",
	Example: `  overcf dns get example.com abc123
  overcf dns get example.com abc123 --json`,
	Args: cobra.ExactArgs(2),
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

		record, err := cf.DNS.Records.Get(ctx, args[1], dns.RecordGetParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "RECORD_NOT_FOUND", err.Error(), nil)
			os.Exit(exitcode.NotFound)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data, err := dnsRecordFromResponse(*record)
			if err != nil {
				formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
				os.Exit(exitcode.GeneralError)
				return nil
			}
			return formatter.Format(os.Stdout, data)
		}

		fmt.Printf("ID:       %s\n", record.ID)
		fmt.Printf("Type:     %s\n", record.Type)
		fmt.Printf("Name:     %s\n", record.Name)
		fmt.Printf("Content:  %s\n", record.Content)
		fmt.Printf("TTL:      %v\n", record.TTL)
		fmt.Printf("Proxied:  %t\n", record.Proxied)

		return nil
	},
}

// dns create flags
var (
	dnsCreateType     string
	dnsCreateName     string
	dnsCreateContent  string
	dnsCreateTTL      int64
	dnsCreateProxied  bool
	dnsCreatePriority int64
	dnsCreatePort     int64
	dnsCreateWeight   int64
	dnsCreateTarget   string
	dnsCreateFlags    int64
	dnsCreateTag      string
	dnsCreateValue    string
	dnsCreateFromJSON string
	dnsCreateStdin    bool
)

var dnsCreateCmd = &cobra.Command{
	Use:   "create <zone>",
	Short: "Create a DNS record",
	Long:  "Create a new DNS record in a zone.",
	Example: `  overcf dns create example.com --type A --name www --content 192.0.2.1
  overcf dns create example.com --type MX --name @ --content mail.example.com --priority 10
  overcf dns create example.com --from-json '{"type":"A","name":"api","content":"10.0.0.1"}'
  echo '{"type":"TXT","name":"_verify","content":"abc123"}' | overcf dns create example.com --stdin`,
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

		// Parse record from flags, JSON, or stdin
		var record types.DNSRecord

		if dnsCreateStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "Failed to read stdin: "+err.Error(), nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
			if err := json.Unmarshal(data, &record); err != nil {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "Invalid JSON: "+err.Error(), nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
		} else if dnsCreateFromJSON != "" {
			if err := json.Unmarshal([]byte(dnsCreateFromJSON), &record); err != nil {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "Invalid JSON: "+err.Error(), nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
		} else {
			if dnsCreateType == "" || dnsCreateName == "" {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "Required flags: --type, --name", nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
			recordType, err := types.ParseRecordType(dnsCreateType)
			if err != nil {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
			if recordType != types.RecordTypeSRV && recordType != types.RecordTypeCAA && dnsCreateContent == "" {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "VALIDATION_ERROR", "Required flags: --content", nil)
				os.Exit(exitcode.ValidationError)
				return nil
			}
			record = types.DNSRecord{
				Type:    recordType,
				Name:    dnsCreateName,
				Content: dnsCreateContent,
				TTL:     dnsCreateTTL,
				Proxied: dnsCreateProxied,
			}
			if cmd.Flags().Changed("priority") {
				record.Priority = &dnsCreatePriority
			}
			if cmd.Flags().Changed("port") {
				record.Port = &dnsCreatePort
			}
			if cmd.Flags().Changed("weight") {
				record.Weight = &dnsCreateWeight
			}
			record.Target = dnsCreateTarget
			if cmd.Flags().Changed("flags") {
				record.Flags = &dnsCreateFlags
			}
			record.Tag = dnsCreateTag
			record.Value = dnsCreateValue
		}

		// Validate record
		if err := record.Validate(); err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		// Build the appropriate record param based on type
		body, err := buildRecordNewBody(record)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		params := dns.RecordNewParams{
			ZoneID: cloudflare.F(zoneID),
			Body:   body,
		}

		created, err := cf.DNS.Records.New(ctx, params)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data := map[string]any{
				"id":      created.ID,
				"type":    created.Type,
				"name":    created.Name,
				"content": created.Content,
				"ttl":     created.TTL,
				"proxied": created.Proxied,
			}
			return formatter.Format(os.Stdout, data)
		}

		fmt.Printf("Created DNS record: %s\n", created.ID)
		fmt.Printf("  %s %s -> %s\n", created.Type, created.Name, created.Content)

		return nil
	},
}

// buildRecordNewBody creates the appropriate record param based on record type
func buildRecordNewBody(record types.DNSRecord) (dns.RecordNewParamsBodyUnion, error) {
	ttl := dns.TTL(record.TTL)
	if record.TTL == 0 {
		ttl = dns.TTL(1) // auto
	}

	switch record.Type {
	case types.RecordTypeA:
		return dns.ARecordParam{
			Type:    cloudflare.F(dns.ARecordTypeA),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
			Proxied: cloudflare.F(record.Proxied),
		}, nil

	case types.RecordTypeAAAA:
		return dns.AAAARecordParam{
			Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
			Proxied: cloudflare.F(record.Proxied),
		}, nil

	case types.RecordTypeCNAME:
		return dns.CNAMERecordParam{
			Type:    cloudflare.F(dns.CNAMERecordTypeCNAME),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
			Proxied: cloudflare.F(record.Proxied),
		}, nil

	case types.RecordTypeMX:
		priority := float64(10)
		if record.Priority != nil {
			priority = float64(*record.Priority)
		}
		return dns.MXRecordParam{
			Type:     cloudflare.F(dns.MXRecordTypeMX),
			Name:     cloudflare.F(record.Name),
			Content:  cloudflare.F(record.Content),
			TTL:      cloudflare.F(ttl),
			Priority: cloudflare.F(priority),
		}, nil

	case types.RecordTypeTXT:
		return dns.TXTRecordParam{
			Type:    cloudflare.F(dns.TXTRecordTypeTXT),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
		}, nil

	case types.RecordTypeNS:
		return dns.NSRecordParam{
			Type:    cloudflare.F(dns.NSRecordTypeNS),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
		}, nil

	case types.RecordTypePTR:
		return dns.PTRRecordParam{
			Type:    cloudflare.F(dns.PTRRecordTypePTR),
			Name:    cloudflare.F(record.Name),
			Content: cloudflare.F(record.Content),
			TTL:     cloudflare.F(ttl),
		}, nil

	case types.RecordTypeSRV:
		if record.Priority == nil || record.Port == nil || record.Weight == nil || record.Target == "" {
			return nil, fmt.Errorf("SRV records require priority, port, weight, and target")
		}
		return dns.SRVRecordParam{
			Type: cloudflare.F(dns.SRVRecordTypeSRV),
			Name: cloudflare.F(record.Name),
			TTL:  cloudflare.F(ttl),
			Data: cloudflare.F(dns.SRVRecordDataParam{
				Priority: cloudflare.F(float64(*record.Priority)),
				Port:     cloudflare.F(float64(*record.Port)),
				Weight:   cloudflare.F(float64(*record.Weight)),
				Target:   cloudflare.F(record.Target),
			}),
		}, nil

	case types.RecordTypeCAA:
		if record.Flags == nil || record.Tag == "" || record.Value == "" {
			return nil, fmt.Errorf("CAA records require flags, tag, and value")
		}
		return dns.CAARecordParam{
			Type: cloudflare.F(dns.CAARecordTypeCAA),
			Name: cloudflare.F(record.Name),
			TTL:  cloudflare.F(ttl),
			Data: cloudflare.F(dns.CAARecordDataParam{
				Flags: cloudflare.F(float64(*record.Flags)),
				Tag:   cloudflare.F(record.Tag),
				Value: cloudflare.F(record.Value),
			}),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported record type for create: %s", record.Type)
	}
}

// dns update flags
var (
	dnsUpdateContent   string
	dnsUpdateTTL       int64
	dnsUpdateProxied   bool
	dnsUpdateNoProxied bool
	dnsUpdatePriority  int64
	dnsUpdatePort      int64
	dnsUpdateWeight    int64
	dnsUpdateTarget    string
	dnsUpdateFlags     int64
	dnsUpdateTag       string
	dnsUpdateValue     string
)

var dnsUpdateCmd = &cobra.Command{
	Use:   "update <zone> <record-id>",
	Short: "Update a DNS record",
	Long:  "Update an existing DNS record. Fetches the current record and applies changes.",
	Example: `  overcf dns update example.com abc123 --content 192.0.2.2
  overcf dns update example.com abc123 --ttl 300
  overcf dns update example.com abc123 --proxied`,
	Args: cobra.ExactArgs(2),
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

		recordID := args[1]

		// First, get the existing record
		existing, err := cf.DNS.Records.Get(ctx, recordID, dns.RecordGetParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "RECORD_NOT_FOUND", err.Error(), nil)
			os.Exit(exitcode.NotFound)
			return nil
		}

		record, err := dnsRecordFromResponse(*existing)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		if dnsUpdateContent != "" {
			record.Content = dnsUpdateContent
		}

		if dnsUpdateTTL > 0 {
			record.TTL = dnsUpdateTTL
		}

		if dnsUpdateProxied {
			record.Proxied = true
		} else if dnsUpdateNoProxied {
			record.Proxied = false
		}

		if cmd.Flags().Changed("priority") {
			record.Priority = &dnsUpdatePriority
		}
		if cmd.Flags().Changed("port") {
			record.Port = &dnsUpdatePort
		}
		if cmd.Flags().Changed("weight") {
			record.Weight = &dnsUpdateWeight
		}
		if cmd.Flags().Changed("target") {
			record.Target = dnsUpdateTarget
		}
		if cmd.Flags().Changed("flags") {
			record.Flags = &dnsUpdateFlags
		}
		if cmd.Flags().Changed("tag") {
			record.Tag = dnsUpdateTag
		}
		if cmd.Flags().Changed("value") {
			record.Value = dnsUpdateValue
		}

		if err := record.Validate(); err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		body, err := buildRecordEditBody(record)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "VALIDATION_ERROR", err.Error(), nil)
			os.Exit(exitcode.ValidationError)
			return nil
		}

		updated, err := cf.DNS.Records.Edit(ctx, recordID, dns.RecordEditParams{
			ZoneID: cloudflare.F(zoneID),
			Body:   body,
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data := map[string]any{
				"id":      updated.ID,
				"type":    updated.Type,
				"name":    updated.Name,
				"content": updated.Content,
				"ttl":     updated.TTL,
				"proxied": updated.Proxied,
			}
			return formatter.Format(os.Stdout, data)
		}

		fmt.Printf("Updated DNS record: %s\n", updated.ID)
		fmt.Printf("  %s %s -> %s\n", updated.Type, updated.Name, updated.Content)

		return nil
	},
}

func buildRecordEditBody(record types.DNSRecord) (dns.RecordEditParamsBodyUnion, error) {
	body, err := buildRecordNewBody(record)
	if err != nil {
		return nil, err
	}
	edited, ok := body.(dns.RecordEditParamsBodyUnion)
	if !ok {
		return nil, fmt.Errorf("unsupported record type for update: %s", record.Type)
	}
	return edited, nil
}

var dnsDeleteCmd = &cobra.Command{
	Use:   "delete <zone> <record-id>",
	Short: "Delete a DNS record",
	Long:  "Delete a DNS record from a zone.",
	Example: `  overcf dns delete example.com abc123
  overcf dns delete example.com abc123 --yes`,
	Args: cobra.ExactArgs(2),
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

		recordID := args[1]

		// Get record details for confirmation
		record, err := cf.DNS.Records.Get(ctx, recordID, dns.RecordGetParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "RECORD_NOT_FOUND", err.Error(), nil)
			os.Exit(exitcode.NotFound)
			return nil
		}

		// Confirm deletion
		target := fmt.Sprintf("%s %s -> %s", record.Type, record.Name, record.Content)
		if !confirm.Destructive("delete", target, flagYes) {
			fmt.Println("Cancelled.")
			return nil
		}

		// Delete the record
		_, err = cf.DNS.Records.Delete(ctx, recordID, dns.RecordDeleteParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data := map[string]any{
				"deleted":   true,
				"record_id": recordID,
			}
			return formatter.Format(os.Stdout, data)
		}

		fmt.Printf("Deleted DNS record: %s\n", recordID)

		return nil
	},
}

func init() {
	// dns list flags
	dnsListCmd.Flags().StringVar(&dnsListType, "type", "", "Filter by record type (A, AAAA, CNAME, etc.)")
	dnsListCmd.Flags().StringVar(&dnsListSearch, "search", "", "Search records by name or content")

	// dns create flags
	dnsCreateCmd.Flags().StringVar(&dnsCreateType, "type", "", "Record type (A, AAAA, CNAME, MX, TXT, etc.)")
	dnsCreateCmd.Flags().StringVar(&dnsCreateName, "name", "", "Record name (@ for root, or subdomain)")
	dnsCreateCmd.Flags().StringVar(&dnsCreateContent, "content", "", "Record content (IP, hostname, or text)")
	dnsCreateCmd.Flags().Int64Var(&dnsCreateTTL, "ttl", 1, "TTL in seconds (1 = auto)")
	dnsCreateCmd.Flags().BoolVar(&dnsCreateProxied, "proxied", false, "Enable Cloudflare proxy")
	dnsCreateCmd.Flags().Int64Var(&dnsCreatePriority, "priority", 0, "Priority for MX/SRV records")
	dnsCreateCmd.Flags().Int64Var(&dnsCreatePort, "port", 0, "Port for SRV records")
	dnsCreateCmd.Flags().Int64Var(&dnsCreateWeight, "weight", 0, "Weight for SRV records")
	dnsCreateCmd.Flags().StringVar(&dnsCreateTarget, "target", "", "Target for SRV records")
	dnsCreateCmd.Flags().Int64Var(&dnsCreateFlags, "flags", 0, "Flags for CAA records")
	dnsCreateCmd.Flags().StringVar(&dnsCreateTag, "tag", "", "Tag for CAA records (issue, issuewild, iodef)")
	dnsCreateCmd.Flags().StringVar(&dnsCreateValue, "value", "", "Value for CAA records")
	dnsCreateCmd.Flags().StringVar(&dnsCreateFromJSON, "from-json", "", "Create from JSON string")
	dnsCreateCmd.Flags().BoolVar(&dnsCreateStdin, "stdin", false, "Read record from stdin as JSON")

	// dns update flags
	dnsUpdateCmd.Flags().StringVar(&dnsUpdateContent, "content", "", "New record content")
	dnsUpdateCmd.Flags().Int64Var(&dnsUpdateTTL, "ttl", 0, "New TTL in seconds")
	dnsUpdateCmd.Flags().BoolVar(&dnsUpdateProxied, "proxied", false, "Enable Cloudflare proxy")
	dnsUpdateCmd.Flags().BoolVar(&dnsUpdateNoProxied, "no-proxied", false, "Disable Cloudflare proxy")
	dnsUpdateCmd.Flags().Int64Var(&dnsUpdatePriority, "priority", 0, "New priority for MX/SRV records")
	dnsUpdateCmd.Flags().Int64Var(&dnsUpdatePort, "port", 0, "New port for SRV records")
	dnsUpdateCmd.Flags().Int64Var(&dnsUpdateWeight, "weight", 0, "New weight for SRV records")
	dnsUpdateCmd.Flags().StringVar(&dnsUpdateTarget, "target", "", "New target for SRV records")
	dnsUpdateCmd.Flags().Int64Var(&dnsUpdateFlags, "flags", 0, "New flags for CAA records")
	dnsUpdateCmd.Flags().StringVar(&dnsUpdateTag, "tag", "", "New tag for CAA records")
	dnsUpdateCmd.Flags().StringVar(&dnsUpdateValue, "value", "", "New value for CAA records")

	dnsCmd.AddCommand(dnsListCmd)
	dnsCmd.AddCommand(dnsGetCmd)
	dnsCmd.AddCommand(dnsCreateCmd)
	dnsCmd.AddCommand(dnsUpdateCmd)
	dnsCmd.AddCommand(dnsDeleteCmd)
}

// truncateContent shortens long content for table display.
func truncateContent(content string) string {
	if len(content) > 40 {
		return content[:37] + "..."
	}
	return content
}
