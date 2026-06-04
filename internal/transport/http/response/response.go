// Package response provides small helpers for writing consistent JSON responses
// and error envelopes across all HTTP handlers.
package response

import (
	"encoding/json"
	"net/http"
)

// FieldError describes a single field-level validation error.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// errorEnvelope is the standard error body shape returned by the API. It is
// always wrapped under a top-level "error" key by WriteError.
type errorEnvelope struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

// WriteJSON sets the Content-Type, writes the status code, and JSON-encodes
// payload. A nil payload writes only headers and status.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes a standard error response of the form
// {"error": {"code": ..., "message": ..., "details": [...]}}.
func WriteError(w http.ResponseWriter, status int, code, message string, details ...FieldError) {
	WriteJSON(w, status, map[string]errorEnvelope{
		"error": {
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
