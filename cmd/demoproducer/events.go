package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Event is one generated record: key and JSON-serialisable value.
type Event struct {
	Key   string
	Value map[string]any
}

// EventGenerator is the base of the extensible demo-event domain: one realistic event per sequence number.
type EventGenerator interface {
	Next(seq int64, rnd *rand.Rand, badRatio float64) Event
}

// Generators is the registry DEMO_STREAMS selects from by name.
var Generators = map[string]EventGenerator{
	"orders":   OrderEvents{},
	"payments": PaymentEvents{},
	"raw":      RawReadings{},
	"local":    LocalPings{},
}

// DemoCatalog holds the vocabularies the generators draw from.
var DemoCatalog = struct {
	OrderTypes      []string
	OrderTypeWeight []int
	Customers       []string
	Currencies      []string
	PayMethods      []string
	PayStatuses     []string
	Sensors         []string
	PingKinds       []string
	CustomerIDBase  int
	LatencyMs       [2]int
	RecentOrders    int64
	MaxItems        int
	MaxAmountCents  int
	CentsPerUnit    float64
}{
	OrderTypes:      []string{"OrderCreated", "OrderPaid", "OrderCancelled", "OrderShipped"},
	OrderTypeWeight: []int{40, 30, 15, 15},
	Customers:       []string{"anna.kowalski", "ben.okafor", "chen.wei", "dina.haddad", "emil.larsen", "fatima.noor", "georg.huber", "hana.sato"},
	Currencies:      []string{"EUR", "USD", "GBP"},
	PayMethods:      []string{"card", "paypal", "sepa"},
	PayStatuses:     []string{"captured", "captured", "captured", "authorized", "failed"},
	Sensors:         []string{"line-1/temp", "line-2/temp", "line-3/pressure", "dock/humidity"},
	PingKinds:       []string{"heartbeat", "inventory-sync", "price-check"},
	CustomerIDBase:  100,
	LatencyMs:       [2]int{20, 420},
	RecentOrders:    50,
	MaxItems:        6,
	MaxAmountCents:  25_000,
	CentsPerUnit:    100,
}

// DemoIDs formats the identifiers of generated events.
var DemoIDs = struct{ Order, Payment, Reading, Ping, Customer, Email string }{
	Order: "ord-%06d", Payment: "pay-%06d", Reading: "rd-%06d", Ping: "loc-%06d", Customer: "cus-%03d", Email: "%s@example.com",
}

// Pick draws one element of a catalog list.
type Pick struct{}

func (Pick) One(rnd *rand.Rand, list []string) string {
	return list[rnd.IntN(len(list))]
}

// Weighted draws an element using integer weights of the same length.
func (Pick) Weighted(rnd *rand.Rand, list []string, weights []int) string {
	total := 0
	for _, w := range weights {
		total += w
	}
	n := rnd.IntN(total)
	for i, w := range weights {
		if n < w {
			return list[i]
		}
		n -= w
	}
	return list[len(list)-1]
}

// Now is the event timestamp in RFC 3339 (UTC, milliseconds).
func Now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// OrderEvents are e-commerce order lifecycle events keyed by orderId (cluster A, demo-orders).
type OrderEvents struct{}

func (OrderEvents) Next(seq int64, rnd *rand.Rand, _ float64) Event {
	c := DemoCatalog
	idx := rnd.IntN(len(c.Customers))
	orderID := fmt.Sprintf(DemoIDs.Order, seq)
	amount := math.Round(float64(rnd.IntN(c.MaxAmountCents)+100)) / c.CentsPerUnit
	return Event{Key: orderID, Value: map[string]any{
		"seq": seq, "orderId": orderID, "eventType": Pick{}.Weighted(rnd, c.OrderTypes, c.OrderTypeWeight),
		"customer": map[string]any{"id": fmt.Sprintf(DemoIDs.Customer, c.CustomerIDBase+idx), "email": fmt.Sprintf(DemoIDs.Email, c.Customers[idx])},
		"amount":   amount, "currency": Pick{}.One(rnd, c.Currencies), "items": rnd.IntN(c.MaxItems) + 1, "ts": Now(),
	}}
}

// PaymentEvents are payment-provider callbacks with snake_case fields and debug data (cluster B, demo-payments).
type PaymentEvents struct{}

func (PaymentEvents) Next(seq int64, rnd *rand.Rand, _ float64) Event {
	c := DemoCatalog
	orderID := fmt.Sprintf(DemoIDs.Order, seq-rnd.Int64N(c.RecentOrders))
	return Event{Key: orderID, Value: map[string]any{
		"seq": seq, "paymentId": fmt.Sprintf(DemoIDs.Payment, seq), "orderId": orderID,
		"amount_cents": rnd.IntN(c.MaxAmountCents) + 100, "currency": Pick{}.One(rnd, c.Currencies),
		"method": Pick{}.One(rnd, c.PayMethods), "status": Pick{}.One(rnd, c.PayStatuses),
		"internal_debug": map[string]any{"gateway": "psp-sim", "latencyMs": c.LatencyMs[0] + rnd.IntN(c.LatencyMs[1]-c.LatencyMs[0])}, "ts": Now(),
	}}
}

// RawReadings are sensor readings of which a fraction is flagged bad:true (cluster B, demo-raw).
type RawReadings struct{}

func (RawReadings) Next(seq int64, rnd *rand.Rand, badRatio float64) Event {
	c := DemoCatalog
	id := fmt.Sprintf(DemoIDs.Reading, seq)
	return Event{Key: id, Value: map[string]any{
		"seq": seq, "id": id, "sensor": Pick{}.One(rnd, c.Sensors),
		"reading": math.Round(rnd.Float64()*1000) / 10, "bad": rnd.Float64() < badRatio, "ts": Now(),
	}}
}

// LocalPings are small service pings for the same-cluster comparison route (cluster B, demo-b-in).
type LocalPings struct{}

func (LocalPings) Next(seq int64, rnd *rand.Rand, _ float64) Event {
	id := fmt.Sprintf(DemoIDs.Ping, seq)
	return Event{Key: id, Value: map[string]any{"seq": seq, "id": id, "kind": Pick{}.One(rnd, DemoCatalog.PingKinds), "ts": Now()}}
}
