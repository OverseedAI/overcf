// Package exitcode defines semantic exit codes for CLI operations.
// These codes enable AI agents and scripts to programmatically
// understand the result of commands without parsing output.
package exitcode

const (
	// Success indicates the command completed successfully.
	Success = 0

	// GeneralError indicates an unspecified error occurred.
	GeneralError = 1

	// AuthError indicates authentication failed (missing or invalid token).
	AuthError = 2

	// NotFound indicates the requested resource does not exist.
	NotFound = 3

	// ValidationError indicates invalid input parameters.
	ValidationError = 4

	// RateLimited indicates the API rate limit was exceeded.
	RateLimited = 5

	// Conflict indicates a resource conflict (e.g., record already exists).
	Conflict = 6

	// NetworkError indicates a network connectivity issue.
	NetworkError = 7
)

// ErrorCode maps error codes to their string representations for JSON output.
var ErrorCode = map[int]string{
	Success:         "SUCCESS",
	GeneralError:    "GENERAL_ERROR",
	AuthError:       "AUTH_ERROR",
	NotFound:        "NOT_FOUND",
	ValidationError: "VALIDATION_ERROR",
	RateLimited:     "RATE_LIMITED",
	Conflict:        "CONFLICT",
	NetworkError:    "NETWORK_ERROR",
}
