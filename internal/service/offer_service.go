package service

import (
	"context"

	"github.com/example/travel-tickets-api/internal/domain"
)

// OfferReader queries stored offers. Implemented by the postgres
// FlightOfferRepository.
type OfferReader interface {
	List(ctx context.Context, filter domain.OfferFilter) ([]domain.FlightOffer, error)
}

// OfferService exposes read access to the collected offers.
type OfferService struct {
	store OfferReader
}

// NewOfferService constructs an OfferService.
func NewOfferService(store OfferReader) *OfferService {
	return &OfferService{store: store}
}

// ListOffers returns stored offers matching the filter.
func (s *OfferService) ListOffers(ctx context.Context, filter domain.OfferFilter) ([]domain.FlightOffer, error) {
	return s.store.List(ctx, filter)
}
