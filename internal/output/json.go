package output

import (
	"encoding/json"
	"io"
)

// JSONFormatter outputs data as JSON, optimized for AI agents and scripts.
type JSONFormatter struct{}

// Format writes data as JSON.
func (f *JSONFormatter) Format(w io.Writer, data any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if _, ok := data.(jsonResponse); ok {
		return encoder.Encode(data)
	}
	return encoder.Encode(NewSuccess(data))
}

// FormatError writes an error as JSON.
func (f *JSONFormatter) FormatError(w io.Writer, code string, message string, details any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(NewError[any](code, message, details))
}

// FormatList writes a list as JSON (converts rows to objects using headers as keys).
func (f *JSONFormatter) FormatList(w io.Writer, headers []string, rows [][]string) error {
	items := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				item[header] = row[i]
			}
		}
		items = append(items, item)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(NewListSuccess(items))
}
