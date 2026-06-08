package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// MessageSender posts a text message to a preconfigured destination. Implemented
// by the telegram client.
type MessageSender interface {
	SendMessage(ctx context.Context, text string) error
}

// ErrNotifierDisabled is returned when a notification is attempted while the
// notifier is disabled (not enabled, or missing bot token / channel).
var ErrNotifierDisabled = errors.New("notifier: telegram notifications are disabled")

// Notifier is the application-level entry point for posting messages to the
// Telegram channel. The rest of the app depends on this, not on the telegram
// client directly — so the delivery mechanism can change without touching callers.
type Notifier struct {
	sender  MessageSender
	enabled bool
	log     *slog.Logger
}

// NewNotifier constructs a Notifier. When enabled is false, Notify is a no-op
// returning ErrNotifierDisabled.
func NewNotifier(sender MessageSender, enabled bool, log *slog.Logger) *Notifier {
	return &Notifier{sender: sender, enabled: enabled, log: log}
}

// Enabled reports whether notifications will actually be delivered.
func (n *Notifier) Enabled() bool {
	return n.enabled && n.sender != nil
}

// Notify posts text to the Telegram channel.
func (n *Notifier) Notify(ctx context.Context, text string) error {
	if !n.Enabled() {
		return ErrNotifierDisabled
	}
	if err := n.sender.SendMessage(ctx, text); err != nil {
		n.log.Error("telegram notify failed", "error", err)
		return fmt.Errorf("notifier: %w", err)
	}
	n.log.Info("telegram message sent")
	return nil
}
