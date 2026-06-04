// Package middleware contains reusable net/http middleware for the API.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ctxKey is a private context key type, preventing collisions with keys defined
// in other packages.
type ctxKey int

const requestIDKey ctxKey = iota

// requestIDHeader is the canonical header used to carry the correlation ID.
const requestIDHeader = "X-Request-ID"

// RequestID ensures every request carries a correlation ID. If the incoming
// X-Request-ID header is empty, a new UUID v4 is generated. The ID is stored in
// the request context and echoed back in the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request ID stored in ctx, or an empty string
// if none is present. Handlers use it to correlate their log lines.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
