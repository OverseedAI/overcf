// Package confirm provides interactive confirmation prompts for destructive operations.
package confirm

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Destructive prompts the user to confirm a destructive operation.
// Returns true if the user confirms, false otherwise.
// If skipConfirm is true, returns true without prompting.
// In non-interactive contexts (piped stdin), returns false unless skipConfirm is true.
func Destructive(action string, target string, skipConfirm bool) bool {
	if skipConfirm {
		return true
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Error: destructive operation requires --yes flag in non-interactive mode\n")
		return false
	}

	fmt.Printf("This will %s: %s\n", action, target)
	fmt.Print("Proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// DestructiveMultiple prompts for confirmation when multiple items are affected.
func DestructiveMultiple(action string, items []string, skipConfirm bool) bool {
	if skipConfirm {
		return true
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Error: destructive operation requires --yes flag in non-interactive mode\n")
		return false
	}

	fmt.Printf("This will %s %d items:\n", action, len(items))
	for _, item := range items {
		fmt.Printf("  - %s\n", item)
	}
	fmt.Print("\nProceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
