package output

import (
	"io"
)

// Formatter defines the interface for output formatting.
type Formatter interface {
	// Format writes the data to the writer in the formatter's format.
	Format(w io.Writer, data any) error

	// FormatError writes an error to the writer.
	FormatError(w io.Writer, code string, message string, details any) error

	// FormatList writes a list of items with headers.
	FormatList(w io.Writer, headers []string, rows [][]string) error
}

// Config holds output configuration options.
type Config struct {
	// Format specifies the output format: "table", "json", or "csv".
	Format string

	// Quiet suppresses non-essential output.
	Quiet bool

	// NoColor disables colored output.
	NoColor bool
}

// New creates a new formatter based on the configuration.
func New(cfg Config) Formatter {
	switch cfg.Format {
	case "json":
		return &JSONFormatter{}
	case "csv":
		return &CSVFormatter{}
	default:
		return &TableFormatter{NoColor: cfg.NoColor}
	}
}
