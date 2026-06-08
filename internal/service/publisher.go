package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/example/travel-tickets-api/internal/domain"
)

// OfferPublishStore lists unpublished offers and marks them published.
// Implemented by the postgres FlightOfferRepository.
type OfferPublishStore interface {
	ListUnpublished(ctx context.Context, limit int) ([]domain.FlightOffer, error)
	MarkPublished(ctx context.Context, id int64) error
}

// Publisher posts not-yet-published offers to the Telegram channel and marks
// them as published. It is run after each collection cycle.
type Publisher struct {
	store     OfferPublishStore
	notifier  *Notifier
	log       *slog.Logger
	batchSize int
	delay     time.Duration
}

// NewPublisher constructs a Publisher.
func NewPublisher(store OfferPublishStore, notifier *Notifier, log *slog.Logger, batchSize int, delay time.Duration) *Publisher {
	return &Publisher{
		store:     store,
		notifier:  notifier,
		log:       log,
		batchSize: batchSize,
		delay:     delay,
	}
}

// PublishResult summarizes a single publishing pass.
type PublishResult struct {
	Fetched   int
	Published int
	Failed    int
}

// PublishPending publishes up to batchSize unpublished offers — one message each,
// pausing delay between sends — and marks each successfully sent offer as
// published. A failure on one offer does not abort the pass.
func (p *Publisher) PublishPending(ctx context.Context) (PublishResult, error) {
	var res PublishResult
	if p.notifier == nil || !p.notifier.Enabled() {
		return res, nil
	}

	offers, err := p.store.ListUnpublished(ctx, p.batchSize)
	if err != nil {
		return res, fmt.Errorf("publisher: list unpublished: %w", err)
	}
	res.Fetched = len(offers)

	var errs []error
	for i, o := range offers {
		if i > 0 {
			if err := sleep(ctx, p.delay); err != nil {
				return res, err // ctx cancelled
			}
		}

		if err := p.notifier.Notify(ctx, formatOfferMessage(o)); err != nil {
			res.Failed++
			errs = append(errs, fmt.Errorf("offer %d: %w", o.ID, err))
			continue
		}

		if err := p.store.MarkPublished(ctx, o.ID); err != nil {
			// The message was sent but we failed to record it; it may be re-sent
			// next cycle. Log and continue (at-least-once delivery).
			res.Failed++
			errs = append(errs, fmt.Errorf("mark offer %d: %w", o.ID, err))
			p.log.Error("publish: mark published failed (message already sent)", "id", o.ID, "error", err)
			continue
		}

		res.Published++
		p.log.Info("offer published",
			"id", o.ID, "origin", o.Origin, "destination", o.Destination,
			"date", o.DepartureDate, "price", o.Price)
	}

	return res, errors.Join(errs...)
}

// --- message formatting ---

// airportNames maps IATA codes to human-readable city names for nicer messages.
var airportNames = map[string]string{
	"OVB": "Новосибирск",
	"AER": "Сочи",
	"KRR": "Краснодар",
	"AAQ": "Анапа",
	"MRV": "Минеральные Воды",
	"SIP": "Симферополь",
}

// formatOfferMessage renders an offer as an HTML message for Telegram. The long
// booking URL is hidden behind a clickable link rather than shown as raw text.
func formatOfferMessage(o domain.FlightOffer) string {
	var b strings.Builder

	fmt.Fprintf(&b, "✈️ <b>%s → %s</b>\n",
		html.EscapeString(airportName(o.Origin)), html.EscapeString(airportName(o.Destination)))
	fmt.Fprintf(&b, "💰 <b>%s</b> · %s\n",
		html.EscapeString(formatPrice(o.Price, o.Currency)), tripTypeText(o.OneWay))
	fmt.Fprintf(&b, "🗓 %s", html.EscapeString(o.DepartureAt.Format("02.01.2006")))
	if o.Airline != "" {
		fmt.Fprintf(&b, " · ✈ %s", html.EscapeString(o.Airline))
	}
	fmt.Fprintf(&b, " · %s", html.EscapeString(transfersText(o.Transfers)))
	if o.Link != "" {
		fmt.Fprintf(&b, "\n\n👉 <a href=\"%s\">Открыть билет</a>", html.EscapeString(o.Link))
	}
	return b.String()
}

func airportName(code string) string {
	if name, ok := airportNames[code]; ok {
		return name
	}
	return code
}

func tripTypeText(oneWay bool) string {
	if oneWay {
		return "в одну сторону"
	}
	return "туда-обратно"
}

func transfersText(n int) string {
	switch {
	case n <= 0:
		return "без пересадок"
	case n == 1:
		return "1 пересадка"
	case n >= 2 && n <= 4:
		return fmt.Sprintf("%d пересадки", n)
	default:
		return fmt.Sprintf("%d пересадок", n)
	}
}

func formatPrice(price int64, currency string) string {
	amount := groupThousands(price)
	switch strings.ToUpper(currency) {
	case "RUB":
		return amount + " ₽"
	case "USD":
		return "$" + amount
	case "EUR":
		return "€" + amount
	default:
		return amount + " " + currency
	}
}

// groupThousands formats n with a space between thousands groups (e.g. 12345 ->
// "12 345").
func groupThousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)

	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(digits[i : i+3])
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	return out
}
