package output

import (
	"encoding/csv"
	"fmt"
	"io"
)

// CSVFormatter outputs data as CSV.
type CSVFormatter struct{}

// Format writes data as CSV (single record).
func (f *CSVFormatter) Format(w io.Writer, data any) error {
	// For single records, output as a simple format
	fmt.Fprintf(w, "%v\n", data)
	return nil
}

// FormatError writes an error (CSV doesn't have a standard error format).
func (f *CSVFormatter) FormatError(w io.Writer, code string, message string, details any) error {
	fmt.Fprintf(w, "error,%s,%s\n", code, message)
	return nil
}

// FormatList writes a list as CSV with headers.
func (f *CSVFormatter) FormatList(w io.Writer, headers []string, rows [][]string) error {
	writer := csv.NewWriter(w)

	// Write header
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Write rows
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}
