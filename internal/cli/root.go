// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/exitcode"
	"github.com/OverseedAI/overcf/internal/output"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	// Global flags
	flagJSON    bool
	flagQuiet   bool
	flagYes     bool
	flagNoColor bool
	flagFormat  string

	// Global output config (set in PersistentPreRun)
	outputConfig output.Config
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:     "overcf",
	Version: version,
	Short:   "Cloudflare CLI - manage DNS and zones",
	Long: `overcf is a command-line interface for managing Cloudflare resources.

Designed for both human operators and AI agents, it provides:
  - Human-readable table output by default
  - JSON output (--json) for programmatic use
  - Smart zone resolution (domain names or IDs)
  - Confirmation prompts for destructive operations

Authentication:
  Set the CLOUDFLARE_API_TOKEN environment variable with your API token.

Examples:
  overcf zone list
  overcf dns list example.com
  overcf dns create example.com --type A --name www --content 192.0.2.1
  overcf dns list example.com --json`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set up output configuration from flags
		format := flagFormat
		if flagJSON {
			format = "json"
		}
		outputConfig = output.Config{
			Format:  format,
			Quiet:   flagQuiet,
			NoColor: flagNoColor,
		}
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Minimal output")
	rootCmd.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "table", "Output format: table, json, csv")

	// Custom version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("overcf %s (commit: %s, built: %s)\n", version, commit, date))

	// Add subcommands
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(zoneCmd)
	rootCmd.AddCommand(dnsCmd)
}

// Execute runs the CLI and returns the exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		formatter := output.New(outputConfig)
		formatter.FormatError(os.Stderr, "GENERAL_ERROR", err.Error(), nil)
		return exitcode.GeneralError
	}
	return exitcode.Success
}

// getFormatter returns the appropriate output formatter based on flags.
func getFormatter() output.Formatter {
	return output.New(outputConfig)
}

func isJSONOutput() bool {
	return outputConfig.Format == "json"
}

func isCSVOutput() bool {
	return outputConfig.Format == "csv"
}

func isTableOutput() bool {
	return outputConfig.Format == "table"
}
