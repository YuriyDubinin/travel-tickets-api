package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/example/travel-tickets-api/internal/transport/http/middleware"
	"github.com/example/travel-tickets-api/internal/transport/http/response"
)

// Notifier is the use case the handler depends on. Implemented by
// service.Notifier.
type Notifier interface {
	Notify(ctx context.Context, text string) error
	Enabled() bool
}

// NotifyHandler exposes a manual way to post a message to the Telegram channel.
// It is a scaffold/test endpoint — add authentication before exposing it
// publicly, since anyone who can reach it can post to the channel.
type NotifyHandler struct {
	notifier Notifier
	log      *slog.Logger
}

// NewNotifyHandler constructs a NotifyHandler.
func NewNotifyHandler(notifier Notifier, log *slog.Logger) *NotifyHandler {
	return &NotifyHandler{notifier: notifier, log: log}
}

type notifyRequest struct {
	Text string `json:"text"`
}

// Send handles POST /api/notify with body {"text": "..."} and posts it to the
// configured Telegram channel.
func (h *NotifyHandler) Send(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !h.notifier.Enabled() {
		response.WriteError(w, http.StatusServiceUnavailable, "TELEGRAM_DISABLED",
			"telegram notifications are not configured")
		return
	}

	var req notifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if req.Text == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "validation failed",
			response.FieldError{Field: "text", Message: "must not be empty"})
		return
	}

	if err := h.notifier.Notify(ctx, req.Text); err != nil {
		h.log.Error("notify failed", "error", err, "request_id", middleware.RequestIDFromContext(ctx))
		response.WriteError(w, http.StatusBadGateway, "NOTIFY_FAILED", "failed to send message")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
