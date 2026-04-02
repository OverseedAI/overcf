package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/client"
	"github.com/OverseedAI/overcf/internal/config"
	"github.com/OverseedAI/overcf/internal/exitcode"
	"github.com/OverseedAI/overcf/internal/output"
	"github.com/OverseedAI/overcf/internal/resolver"
	"github.com/OverseedAI/overcf/internal/types"
)

var dnsExportCmd = &cobra.Command{
	Use:   "export <zone>",
	Short: "Export DNS records",
	Long:  "Export DNS records in JSON or CSV format.",
	Example: `  overcf dns export example.com --json
  overcf dns export example.com --format csv > records.csv`,
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

		records, err := listAllRecords(ctx, cf, zoneID)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		out := make([]types.DNSRecord, 0, len(records))
		for _, r := range records {
			record, err := dnsRecordFromResponse(r)
			if err != nil {
				formatter := getFormatter()
				formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
				os.Exit(exitcode.GeneralError)
				return nil
			}
			out = append(out, record)
		}

		formatter := getFormatter()

		if isJSONOutput() {
			return formatter.Format(os.Stdout, output.NewListSuccess(out))
		}

		headers := []string{
			"ID", "TYPE", "NAME", "CONTENT", "TTL", "PROXIED",
			"PRIORITY", "PORT", "WEIGHT", "TARGET", "FLAGS", "TAG", "VALUE",
		}

		rows := make([][]string, 0, len(out))
		for _, record := range out {
			id := record.ID
			content := record.Content
			ttl := strconv.FormatInt(record.TTL, 10)
			proxied := strconv.FormatBool(record.Proxied)
			if isTableOutput() {
				if record.ID != "" {
					id = truncateID(record.ID)
				}
				if record.Content != "" {
					content = truncateContent(record.Content)
				}
				if record.TTL == 1 {
					ttl = "auto"
				}
				proxied = record.ProxiedString()
			}

			rows = append(rows, []string{
				id,
				string(record.Type),
				record.Name,
				content,
				ttl,
				proxied,
				intPtrString(record.Priority),
				intPtrString(record.Port),
				intPtrString(record.Weight),
				record.Target,
				intPtrString(record.Flags),
				record.Tag,
				record.Value,
			})
		}

		return formatter.FormatList(os.Stdout, headers, rows)
	},
}

func init() {
	dnsCmd.AddCommand(dnsExportCmd)
}

func intPtrString(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
