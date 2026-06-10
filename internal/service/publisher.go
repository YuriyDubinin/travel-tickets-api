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

// PublishPending publishes ALL not-yet-published offers — one message each,
// pausing delay between sends — and marks each successfully sent offer as
// published. Offers are fetched in pages of batchSize until none remain, so a
// large backlog is fully drained in a single pass. A failure on one offer does
// not abort the pass; failed offers stay unpublished and are retried next cycle.
func (p *Publisher) PublishPending(ctx context.Context) (PublishResult, error) {
	var res PublishResult
	if p.notifier == nil || !p.notifier.Enabled() {
		return res, nil
	}

	var errs []error
	attempted := make(map[int64]bool) // guards against re-fetching offers that failed this pass

	for {
		offers, err := p.store.ListUnpublished(ctx, p.batchSize)
		if err != nil {
			errs = append(errs, fmt.Errorf("publisher: list unpublished: %w", err))
			break
		}

		// Skip offers already attempted this pass: a failed send stays unpublished
		// and would otherwise be fetched again forever.
		fresh := make([]domain.FlightOffer, 0, len(offers))
		for _, o := range offers {
			if !attempted[o.ID] {
				fresh = append(fresh, o)
			}
		}
		if len(fresh) == 0 {
			break
		}

		for _, o := range fresh {
			attempted[o.ID] = true

			if res.Fetched > 0 { // pause between messages, not before the first
				if err := sleep(ctx, p.delay); err != nil {
					return res, err // ctx cancelled
				}
			}
			res.Fetched++

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
	}

	return res, errors.Join(errs...)
}

// Announce posts a one-time intro message describing what the bot publishes:
// the routes, the departure-date window, and how often prices are checked.
func (p *Publisher) Announce(ctx context.Context, origin string, destinations []string, interval time.Duration) error {
	if p.notifier == nil || !p.notifier.Enabled() {
		return nil
	}
	return p.notifier.Notify(ctx, formatAnnounceMessage(origin, destinations, interval))
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
	"CXR": "Нячанг",
	"NHA": "Нячанг",
	"SGN": "Хошимин",
	"HAN": "Ханой",
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

// formatAnnounceMessage renders the one-time startup intro for the channel.
func formatAnnounceMessage(origin string, destinations []string, interval time.Duration) string {
	names := make([]string, 0, len(destinations))
	for _, d := range destinations {
		names = append(names, airportName(d))
	}

	var b strings.Builder
	b.WriteString("🔥 <b>Самые выгодные авиабилеты</b>\n\n")
	fmt.Fprintf(&b, "Теперь отслеживаем перелёты по направлениям:\n✈️ <b>%s → %s</b>\n\n",
		html.EscapeString(airportName(origin)), html.EscapeString(strings.Join(names, ", ")))
	fmt.Fprintf(&b, "🗓 Вылеты в ближайшие <b>%d %s</b>\n",
		collectionWindowDays, plural(collectionWindowDays, "день", "дня", "дней"))
	fmt.Fprintf(&b, "🔄 Обновляем раз в <b>%s</b>", html.EscapeString(humanizeInterval(interval)))
	return b.String()
}

// humanizeInterval renders a duration as a short Russian phrase (e.g. "5 минут").
func humanizeInterval(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		h := int(d / time.Hour)
		return fmt.Sprintf("%d %s", h, plural(h, "час", "часа", "часов"))
	case d >= time.Minute && d%time.Minute == 0:
		m := int(d / time.Minute)
		return fmt.Sprintf("%d %s", m, plural(m, "минуту", "минуты", "минут"))
	default:
		return d.String()
	}
}

// plural picks the Russian plural form for n (one / few / many).
func plural(n int, one, few, many string) string {
	if mod100 := n % 100; mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
