// Package transporthttp wires the HTTP transport layer: the server lifecycle
// and the chi router.
package transporthttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// shutdownTimeout bounds how long graceful shutdown waits for in-flight
// requests to drain.
const shutdownTimeout = 10 * time.Second

// HTTPConfig carries the settings needed to construct the HTTP server.
type HTTPConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Server wraps *http.Server with graceful-shutdown lifecycle management.
type Server struct {
	srv *http.Server
	log *slog.Logger
}

// NewServer constructs a Server listening on ":" + cfg.Port with the configured
// timeouts.
func NewServer(cfg HTTPConfig, handler http.Handler, log *slog.Logger) *Server {
	return &Server{
		srv: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		log: log,
	}
}

// Run starts the server and blocks until ctx is cancelled or the server fails.
// On cancellation it performs a graceful shutdown bounded by shutdownTimeout.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info("http server starting", "addr", s.srv.Addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error("http server shutdown error", "error", err)
			return err
		}

		s.log.Info("http server stopped")
		return nil
	}
}
