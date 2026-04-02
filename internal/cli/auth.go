package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/OverseedAI/overcf/internal/client"
	"github.com/OverseedAI/overcf/internal/config"
	"github.com/OverseedAI/overcf/internal/exitcode"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  "Commands for managing Cloudflare API authentication.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Set up API token authentication",
	Long: `Configure authentication with a Cloudflare API token.

The recommended approach is to set the CLOUDFLARE_API_TOKEN environment variable:

  export CLOUDFLARE_API_TOKEN=your_token_here

You can create an API token at:
  https://dash.cloudflare.com/profile/api-tokens`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("To authenticate, set the CLOUDFLARE_API_TOKEN environment variable:")
		fmt.Println()
		fmt.Println("  export CLOUDFLARE_API_TOKEN=your_token_here")
		fmt.Println()
		fmt.Println("Create an API token at:")
		fmt.Println("  https://dash.cloudflare.com/profile/api-tokens")
		fmt.Println()
		fmt.Println("For DNS management, your token needs the following permissions:")
		fmt.Println("  - Zone:Zone:Read")
		fmt.Println("  - Zone:DNS:Edit")
		return nil
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication context",
	Long:  "Display information about the currently authenticated user and token.",
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

		// Verify token by making an API call
		ctx := context.Background()
		user, err := cf.User.Get(ctx)
		if err != nil {
			formatter := getFormatter()
			formatter.FormatError(os.Stderr, "AUTH_INVALID", "Failed to verify token: "+err.Error(), nil)
			os.Exit(exitcode.AuthError)
			return nil
		}

		formatter := getFormatter()

		if isJSONOutput() {
			data := map[string]any{
				"id":         user.ID,
				"first_name": user.FirstName,
				"last_name":  user.LastName,
				"country":    user.Country,
				"suspended":  user.Suspended,
			}
			formatter.Format(os.Stdout, data)
		} else {
			fmt.Printf("ID:       %s\n", user.ID)
			if user.FirstName != "" || user.LastName != "" {
				fmt.Printf("Name:     %s %s\n", user.FirstName, user.LastName)
			}
			if user.Country != "" {
				fmt.Printf("Country:  %s\n", user.Country)
			}
			fmt.Printf("Token:    ****%s (configured via env)\n", maskToken(cfg.APIToken))
		}

		return nil
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authWhoamiCmd)
}

// maskToken returns the last 4 characters of a token for display.
func maskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return token[len(token)-4:]
}
