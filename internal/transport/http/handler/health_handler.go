// Package handler contains the HTTP handlers for the API. Each handler depends
// only on the services it needs and writes responses via the response package.
package handler

import (
	"net/http"
	"time"

	"github.com/example/travel-tickets-api/internal/transport/http/response"
)

// HealthHandler serves a process-level health check. It has no dependencies and
// performs no I/O: a successful response simply means the process is alive and
// able to serve HTTP. It deliberately does not ping the database.
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Ping responds 200 OK with the current UTC time in RFC3339 format.
func (h *HealthHandler) Ping(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
