package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/example/travel-tickets-api/internal/transport/http/response"
)

// Recover returns a middleware that recovers from panics in downstream
// handlers, logs the error together with a stack trace, and responds with a
// 500 error envelope.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"error", rec,
						"request_id", RequestIDFromContext(r.Context()),
						"stack", string(debug.Stack()),
					)
					response.WriteError(w, http.StatusInternalServerError,
						"INTERNAL_ERROR", "internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
