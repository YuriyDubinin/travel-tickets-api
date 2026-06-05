// Package domain holds the core business entities, free of any transport,
// storage, or third-party dependencies.
//
// Note on routes: there are currently no flights from Novosibirsk (OVB) to
// Simferopol (SIP) — the only civilian airport in Crimea has been closed since
// 2022. The service therefore collects offers to the nearest "feeder" airports
// (Sochi/AER, Krasnodar/KRR, Anapa/AAQ, Mineralnye Vody/MRV); the final leg to
// Crimea is by ground transport (train/bus).
package domain

import "time"

// FlightOffer is a single cached flight price for a route on a given departure
// date, as surfaced by the Aviasales Data API.
type FlightOffer struct {
	Origin             string
	Destination        string
	OriginAirport      string
	DestinationAirport string
	DepartureAt        time.Time
	DepartureDate      string // YYYY-MM-DD; dedup key together with the route + one_way
	ReturnAt           *time.Time
	Price              int64
	Currency           string
	Airline            string
	FlightNumber       string
	Transfers          int
	Duration           int // minutes
	Link               string
	OneWay             bool
	Source             string
	CollectedAt        time.Time
}

// OfferFilter narrows a flight-offer query. Empty string fields are ignored; a
// non-positive Limit lets the repository apply its default.
type OfferFilter struct {
	Origin      string
	Destination string
	Limit       int
}

// Route is an origin/destination airport pair.
type Route struct {
	Origin      string
	Destination string
}

// String renders the route in "OVB-AER" form.
func (r Route) String() string {
	return r.Origin + "-" + r.Destination
}
