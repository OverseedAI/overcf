package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/zones"
	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/client"
	"github.com/OverseedAI/overcf/internal/config"
	"github.com/OverseedAI/overcf/internal/exitcode"
	"github.com/OverseedAI/overcf/internal/output"
	"github.com/OverseedAI/overcf/internal/resolver"
	"github.com/OverseedAI/overcf/internal/types"
)

var zoneCmd = &cobra.Command{
	Use:   "zone",
	Short: "Manage DNS zones",
	Long:  "Commands for listing and viewing Cloudflare DNS zones.",
}

var (
	zoneListStatus string
	zoneListName   string
)

var zoneListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all zones",
	Long:  "List all DNS zones in your Cloudflare account.",
	Example: `  overcf zone list
  overcf zone list --status active
  overcf zone list --json`,
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
		params := zones.ZoneListParams{}

		if zoneListName != "" {
			params.Name = cloudflare.F(zoneListName)
		}
		if zoneListStatus != "" {
			params.Status = cloudflare.F(zones.ZoneListParamsStatus(zoneListStatus))
		}

		// Use auto-paging to get all zones
		var allZones []zones.Zone
		iter := cf.Zones.ListAutoPaging(ctx, params)
		for iter.Next() {
			allZones = append(allZones, iter.Current())
		}
		if err := iter.Err(); err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
			os.Exit(exitcode.GeneralError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			out := make([]types.Zone, 0, len(allZones))
			for _, z := range allZones {
				out = append(out, types.Zone{
					ID:          z.ID,
					Name:        z.Name,
					Status:      string(z.Status),
					Plan:        z.Plan.Name,
					NameServers: z.NameServers,
				})
			}
			return formatter.Format(os.Stdout, output.NewListSuccess(out))
		}

		headers := []string{"ID", "NAME", "STATUS", "PLAN"}
		rows := make([][]string, 0, len(allZones))
		for _, z := range allZones {
			planName := ""
			if z.Plan.Name != "" {
				planName = z.Plan.Name
			}
			id := z.ID
			if isTableOutput() {
				id = truncateID(z.ID)
			}
			rows = append(rows, []string{
				id,
				z.Name,
				string(z.Status),
				planName,
			})
		}

		return formatter.FormatList(os.Stdout, headers, rows)
	},
}

var zoneGetCmd = &cobra.Command{
	Use:   "get <zone>",
	Short: "Get zone details",
	Long:  "Get detailed information about a specific zone.",
	Example: `  overcf zone get example.com
  overcf zone get 023e105f4ecef8ad9ca31a8372d0c353
  overcf zone get example.com --json`,
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

		zone, err := cf.Zones.Get(ctx, zones.ZoneGetParams{
			ZoneID: cloudflare.F(zoneID),
		})
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "ZONE_NOT_FOUND", err.Error(), nil)
			os.Exit(exitcode.NotFound)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data := map[string]any{
				"id":           zone.ID,
				"name":         zone.Name,
				"status":       zone.Status,
				"plan":         zone.Plan.Name,
				"name_servers": zone.NameServers,
			}
			return formatter.Format(os.Stdout, data)
		}

		fmt.Printf("ID:           %s\n", zone.ID)
		fmt.Printf("Name:         %s\n", zone.Name)
		fmt.Printf("Status:       %s\n", zone.Status)
		fmt.Printf("Plan:         %s\n", zone.Plan.Name)
		fmt.Printf("Name Servers:\n")
		for _, ns := range zone.NameServers {
			fmt.Printf("  - %s\n", ns)
		}

		return nil
	},
}

func init() {
	zoneListCmd.Flags().StringVar(&zoneListStatus, "status", "", "Filter by status: active, pending, moved")
	zoneListCmd.Flags().StringVar(&zoneListName, "name", "", "Filter by name")

	zoneCmd.AddCommand(zoneListCmd)
	zoneCmd.AddCommand(zoneGetCmd)
}

// truncateID shortens a zone ID for table display.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}
