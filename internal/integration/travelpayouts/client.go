// Package travelpayouts is an outbound client for the Travelpayouts
// (Aviasales) Data API. It maps API responses into domain.FlightOffer values.
package travelpayouts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/example/travel-tickets-api/internal/domain"
)

const (
	defaultBaseURL     = "https://api.travelpayouts.com"
	defaultCurrency    = "rub"
	defaultTimeout     = 30 * time.Second
	pricesForDatesPath = "/aviasales/v3/prices_for_dates"
	maxResponseBytes   = 8 << 20 // 8 MiB safety cap on the response body
	// aviasalesWebBase is prepended to the API's relative search links so the
	// link stored in the DB is a complete, clickable URL.
	aviasalesWebBase = "https://www.aviasales.ru"
)

// Config configures the client. Kept local so this package does not depend on
// the application's config package.
type Config struct {
	BaseURL  string
	Token    string
	Marker   string
	Currency string
	Timeout  time.Duration
}

// Client talks to the Travelpayouts Data API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	marker     string
	currency   string
}

// NewClient builds a Client, applying sensible defaults for empty settings.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	currency := cfg.Currency
	if currency == "" {
		currency = defaultCurrency
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      cfg.Token,
		marker:     cfg.Marker,
		currency:   currency,
	}
}

// Params are the inputs to a PricesForDates query.
type Params struct {
	Origin      string
	Destination string
	DepartureAt string // "YYYY-MM" (whole month) or "YYYY-MM-DD"
	OneWay      bool
	Limit       int
}

// pricesResponse mirrors the prices_for_dates JSON envelope.
type pricesResponse struct {
	Success  bool         `json:"success"`
	Error    string       `json:"error"`
	Currency string       `json:"currency"`
	Data     []priceEntry `json:"data"`
}

// priceEntry mirrors a single cached offer.
type priceEntry struct {
	Origin             string `json:"origin"`
	Destination        string `json:"destination"`
	OriginAirport      string `json:"origin_airport"`
	DestinationAirport string `json:"destination_airport"`
	Price              int64  `json:"price"`
	Airline            string `json:"airline"`
	FlightNumber       string `json:"flight_number"`
	DepartureAt        string `json:"departure_at"`
	ReturnAt           string `json:"return_at"`
	Transfers          int    `json:"transfers"`
	Duration           int    `json:"duration"`
	Link               string `json:"link"`
}

// PricesForDates fetches cached prices for a single route. An empty result set
// is returned as an empty slice (not an error) — this is the expected outcome
// for routes that simply have no flights (e.g. OVB→SIP).
func (c *Client) PricesForDates(ctx context.Context, params Params) ([]domain.FlightOffer, error) {
	u, err := url.Parse(c.baseURL + pricesForDatesPath)
	if err != nil {
		return nil, fmt.Errorf("travelpayouts: parse url: %w", err)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	q := u.Query()
	q.Set("origin", params.Origin)
	q.Set("destination", params.Destination)
	q.Set("departure_at", params.DepartureAt)
	q.Set("currency", c.currency)
	q.Set("sorting", "price")
	q.Set("direct", "false")
	q.Set("one_way", strconv.FormatBool(params.OneWay))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("page", "1")
	if c.marker != "" {
		q.Set("marker", c.marker)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("travelpayouts: new request: %w", err)
	}
	// Token goes in the header, never in the query string or logs.
	req.Header.Set("X-Access-Token", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("travelpayouts: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("travelpayouts: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("travelpayouts: unexpected status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed pricesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("travelpayouts: decode body: %w", err)
	}
	if !parsed.Success && parsed.Error != "" {
		return nil, fmt.Errorf("travelpayouts: api error: %s", parsed.Error)
	}

	currency := parsed.Currency
	if currency == "" {
		currency = c.currency
	}
	return transform(parsed.Data, currency, params.OneWay, time.Now().UTC()), nil
}

// transform maps API entries into domain offers, skipping entries with an
// unparseable departure timestamp.
func transform(entries []priceEntry, currency string, oneWay bool, collectedAt time.Time) []domain.FlightOffer {
	currency = strings.ToUpper(currency)
	offers := make([]domain.FlightOffer, 0, len(entries))
	for _, e := range entries {
		departureAt, ok := parseTime(e.DepartureAt)
		if !ok {
			continue
		}

		var returnAt *time.Time
		if t, ok := parseTime(e.ReturnAt); ok {
			returnAt = &t
		}

		offers = append(offers, domain.FlightOffer{
			Origin:             e.Origin,
			Destination:        e.Destination,
			OriginAirport:      e.OriginAirport,
			DestinationAirport: e.DestinationAirport,
			DepartureAt:        departureAt,
			DepartureDate:      departureAt.Format("2006-01-02"),
			ReturnAt:           returnAt,
			Price:              e.Price,
			Currency:           currency,
			Airline:            e.Airline,
			FlightNumber:       e.FlightNumber,
			Transfers:          e.Transfers,
			Duration:           e.Duration,
			Link:               absoluteLink(e.Link),
			OneWay:             oneWay,
			Source:             "travelpayouts",
			CollectedAt:        collectedAt,
		})
	}
	return offers
}

// parseTime accepts RFC3339 timestamps or bare YYYY-MM-DD dates.
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// truncate shortens s for safe inclusion in error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// absoluteLink turns the API's relative search link into a complete aviasales.ru
// URL. An already-absolute link is returned unchanged; an empty link stays empty.
func absoluteLink(link string) string {
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	return aviasalesWebBase + link
}
