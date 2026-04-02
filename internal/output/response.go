// Package output provides formatters for CLI output in various formats.
package output

// Response is the unified response structure for JSON output.
// It provides a consistent schema for AI agents to parse.
type Response[T any] struct {
	// Success indicates whether the operation succeeded.
	Success bool `json:"success"`

	// Data contains the operation result on success.
	Data T `json:"data,omitempty"`

	// Error contains error details on failure.
	Error *ErrorInfo `json:"error,omitempty"`
}

// ListResponse wraps list operations with count metadata.
type ListResponse[T any] struct {
	// Success indicates whether the operation succeeded.
	Success bool `json:"success"`

	// Data contains the list of items.
	Data []T `json:"data"`

	// Count is the number of items in the list.
	Count int `json:"count"`

	// Error contains error details on failure.
	Error *ErrorInfo `json:"error,omitempty"`
}

// ErrorInfo provides structured error information for JSON output.
type ErrorInfo struct {
	// Code is a machine-readable error code (e.g., "ZONE_NOT_FOUND").
	Code string `json:"code"`

	// Message is a human-readable error description.
	Message string `json:"message"`

	// Details contains additional error context.
	Details any `json:"details,omitempty"`
}

type jsonResponse interface {
	isJSONResponse()
}

func (Response[T]) isJSONResponse() {}

func (ListResponse[T]) isJSONResponse() {}

// NewSuccess creates a successful response.
func NewSuccess[T any](data T) Response[T] {
	return Response[T]{
		Success: true,
		Data:    data,
	}
}

// NewListSuccess creates a successful list response.
func NewListSuccess[T any](data []T) ListResponse[T] {
	return ListResponse[T]{
		Success: true,
		Data:    data,
		Count:   len(data),
	}
}

// NewError creates an error response.
func NewError[T any](code, message string, details any) Response[T] {
	return Response[T]{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}
