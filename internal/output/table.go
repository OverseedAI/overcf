package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// TableFormatter outputs data as human-readable tables.
type TableFormatter struct {
	NoColor bool
}

// Format writes data as a simple key-value display.
func (f *TableFormatter) Format(w io.Writer, data any) error {
	// For simple objects, we'll display as key: value pairs
	// This is a simplified implementation; expand as needed
	fmt.Fprintf(w, "%v\n", data)
	return nil
}

// FormatError writes an error in human-readable format.
func (f *TableFormatter) FormatError(w io.Writer, code string, message string, details any) error {
	fmt.Fprintf(w, "Error: %s\n", message)
	if details != nil {
		fmt.Fprintf(w, "Details: %v\n", details)
	}
	return nil
}

// FormatList writes a list as a formatted table.
func (f *TableFormatter) FormatList(w io.Writer, headers []string, rows [][]string) error {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No records found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	// Write header
	headerLine := strings.Join(headers, "\t")
	fmt.Fprintln(tw, headerLine)

	// Write rows
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	return tw.Flush()
}
