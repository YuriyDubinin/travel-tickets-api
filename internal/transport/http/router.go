package transporthttp

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/example/travel-tickets-api/internal/transport/http/handler"
	mw "github.com/example/travel-tickets-api/internal/transport/http/middleware"
)

// Deps carries the dependencies needed to build the router. Add new handlers
// and shared services here as the API grows.
type Deps struct {
	Logger        *slog.Logger
	HealthHandler *handler.HealthHandler
}

// NewRouter wires middleware and routes into a single http.Handler.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	// Global middleware, applied in order. RequestID runs first so the
	// correlation ID is available to the logger and recoverer; CORS runs last.
	r.Use(mw.RequestID)
	r.Use(mw.Logger(deps.Logger))
	r.Use(mw.Recover(deps.Logger))
	r.Use(mw.CORS)

	r.Route("/api", func(r chi.Router) {
		// Public routes (no authentication required).
		r.Get("/ping", deps.HealthHandler.Ping)

		// ── Protected routes ────────────────────────────────────────────
		// When authentication is introduced, mount authenticated resources
		// inside a sub-group that applies the auth middleware, e.g.:
		//
		//   r.Group(func(r chi.Router) {
		//       r.Use(mw.Auth(deps.TokenVerifier))
		//       r.Mount("/trips", tripsRouter(deps))
		//       r.Mount("/tickets", ticketsRouter(deps))
		//   })
		//
		// Each new resource follows the same pipeline:
		//   migration → domain → repository → service → handler → DTO →
		//   route registered here → dependency injected in cmd/api/main.go.
	})

	return r
}
