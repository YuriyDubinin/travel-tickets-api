package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/travel-tickets-api/internal/domain"
	"github.com/example/travel-tickets-api/internal/transport/http/middleware"
	"github.com/example/travel-tickets-api/internal/transport/http/response"
)

// OfferLister is the read use case the handler depends on. Implemented by
// service.OfferService.
type OfferLister interface {
	ListOffers(ctx context.Context, filter domain.OfferFilter) ([]domain.FlightOffer, error)
}

// OffersHandler serves the collected flight offers.
type OffersHandler struct {
	offers OfferLister
	log    *slog.Logger
}

// NewOffersHandler constructs an OffersHandler.
func NewOffersHandler(offers OfferLister, log *slog.Logger) *OffersHandler {
	return &OffersHandler{offers: offers, log: log}
}

// offerDTO is the JSON representation of a flight offer.
type offerDTO struct {
	Origin             string  `json:"origin"`
	Destination        string  `json:"destination"`
	OriginAirport      string  `json:"origin_airport"`
	DestinationAirport string  `json:"destination_airport"`
	DepartureAt        string  `json:"departure_at"`
	DepartureDate      string  `json:"departure_date"`
	ReturnAt           *string `json:"return_at,omitempty"`
	Price              int64   `json:"price"`
	Currency           string  `json:"currency"`
	Airline            string  `json:"airline"`
	FlightNumber       string  `json:"flight_number"`
	Transfers          int     `json:"transfers"`
	Duration           int     `json:"duration"`
	OneWay             bool    `json:"one_way"`
	Link               string  `json:"link"`
	Source             string  `json:"source"`
	CollectedAt        string  `json:"collected_at"`
}

// List handles GET /api/offers. Supported query params: origin, destination,
// limit. It returns {"data": [...], "count": N}.
func (h *OffersHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	filter := domain.OfferFilter{
		Origin:      strings.ToUpper(strings.TrimSpace(q.Get("origin"))),
		Destination: strings.ToUpper(strings.TrimSpace(q.Get("destination"))),
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			filter.Limit = n
		}
	}

	offers, err := h.offers.ListOffers(ctx, filter)
	if err != nil {
		h.log.Error("list offers failed",
			"error", err,
			"request_id", middleware.RequestIDFromContext(ctx),
		)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list offers")
		return
	}

	dtos := make([]offerDTO, 0, len(offers))
	for _, o := range offers {
		dtos = append(dtos, toOfferDTO(o))
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  dtos,
		"count": len(dtos),
	})
}

// toOfferDTO maps a domain offer into its JSON representation.
func toOfferDTO(o domain.FlightOffer) offerDTO {
	dto := offerDTO{
		Origin:             o.Origin,
		Destination:        o.Destination,
		OriginAirport:      o.OriginAirport,
		DestinationAirport: o.DestinationAirport,
		DepartureAt:        o.DepartureAt.Format(time.RFC3339),
		DepartureDate:      o.DepartureDate,
		Price:              o.Price,
		Currency:           o.Currency,
		Airline:            o.Airline,
		FlightNumber:       o.FlightNumber,
		Transfers:          o.Transfers,
		Duration:           o.Duration,
		OneWay:             o.OneWay,
		Link:               fullLink(o.Link),
		Source:             o.Source,
		CollectedAt:        o.CollectedAt.Format(time.RFC3339),
	}
	if o.ReturnAt != nil {
		s := o.ReturnAt.Format(time.RFC3339)
		dto.ReturnAt = &s
	}
	return dto
}

// fullLink turns the API's relative search link into an absolute aviasales.ru URL.
func fullLink(link string) string {
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	return "https://www.aviasales.ru" + link
}
