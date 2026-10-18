package kafka

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// DemoOrderItem is one line of a demo order.
type DemoOrderItem struct {
	SKU   string  `json:"sku"`
	Qty   int     `json:"qty"`
	Price float64 `json:"price"`
}

// DemoOrderEvent is the JSON value of an `order-events` record (the shape the web app's samples use).
type DemoOrderEvent struct {
	EventType string          `json:"eventType"`
	OrderID   string          `json:"orderId"`
	CustID    string          `json:"cust_id"`
	Email     string          `json:"email"`
	Amount    float64         `json:"amount"`
	Currency  string          `json:"currency"`
	Country   string          `json:"country"`
	Items     []DemoOrderItem `json:"items"`
	CreatedAt string          `json:"createdAt"`
}

// DemoHeader names the headers of demo records.
type DemoHeader string

const (
	DemoHeaderSource    DemoHeader = "source"
	DemoHeaderEventType DemoHeader = "event-type"
)

// DemoSourceValue marks every demo record's origin.
type DemoSourceValue string

const DemoSource DemoSourceValue = "demo"

// DemoFormat holds the id/email/sku templates of demo records.
type DemoFormat string

const (
	DemoOrderIDFormat DemoFormat = "ord_%06d"
	DemoCustIDFormat  DemoFormat = "c_%04d"
	DemoEmailFormat   DemoFormat = "customer%04d@example.com"
	DemoSKUFormat     DemoFormat = "SKU-%04d"
)

// DemoOrderCatalog lists the values a demo order is drawn from.
type DemoOrderCatalog struct {
	EventTypes []string
	Currencies []string
	Countries  []string
	Customers  int
	SKUs       int
	SKUBase    int
	MaxItems   int
	MaxQty     int
	MaxPrice   float64
}

// DefaultDemoOrderCatalog is the order domain of the demo data.
func DefaultDemoOrderCatalog() DemoOrderCatalog {
	return DemoOrderCatalog{
		EventTypes: []string{"order.created", "order.created", "order.paid", "order.paid", "order.shipped", "order.cancelled"},
		Currencies: []string{"EUR", "USD", "GBP"},
		Countries:  []string{"DE", "FR", "US", "GB", "NL", "ES", "PL"},
		Customers:  500, SKUs: 200, SKUBase: 1000, MaxItems: 4, MaxQty: 3, MaxPrice: 250,
	}
}

// DemoOrderGenerator produces deterministic order events: record i is the same in every process.
type DemoOrderGenerator struct {
	seed    uint64
	next    int
	catalog DemoOrderCatalog
}

// NewDemoOrderGenerator starts at record index start.
func NewDemoOrderGenerator(seed uint64, start int) *DemoOrderGenerator {
	return &DemoOrderGenerator{seed: seed, next: start, catalog: DefaultDemoOrderCatalog()}
}

// Next returns the next record, keyed by order id and stamped at.
func (g *DemoOrderGenerator) Next(at time.Time) *kgo.Record {
	event := g.Event(g.next, at)
	g.next++
	value, _ := json.Marshal(event)
	return &kgo.Record{
		Topic: string(DemoOrderEvents), Key: []byte(event.OrderID), Value: value, Timestamp: at,
		Headers: []kgo.RecordHeader{
			{Key: string(DemoHeaderSource), Value: []byte(DemoSource)},
			{Key: string(DemoHeaderEventType), Value: []byte(event.EventType)},
		},
	}
}

// Event builds order event i.
func (g *DemoOrderGenerator) Event(i int, at time.Time) DemoOrderEvent {
	r := rand.New(rand.NewPCG(g.seed, uint64(i)))
	cat := g.catalog
	cust := r.IntN(cat.Customers)
	items := g.items(r)
	return DemoOrderEvent{
		EventType: cat.EventTypes[r.IntN(len(cat.EventTypes))],
		OrderID:   fmt.Sprintf(string(DemoOrderIDFormat), i+1),
		CustID:    fmt.Sprintf(string(DemoCustIDFormat), cust),
		Email:     fmt.Sprintf(string(DemoEmailFormat), cust),
		Amount:    DemoMoney(items),
		Currency:  cat.Currencies[r.IntN(len(cat.Currencies))],
		Country:   cat.Countries[r.IntN(len(cat.Countries))],
		Items:     items,
		CreatedAt: at.UTC().Format(time.RFC3339Nano),
	}
}

func (g *DemoOrderGenerator) items(r *rand.Rand) []DemoOrderItem {
	cat := g.catalog
	out := make([]DemoOrderItem, 1+r.IntN(cat.MaxItems))
	for i := range out {
		out[i] = DemoOrderItem{
			SKU:   fmt.Sprintf(string(DemoSKUFormat), cat.SKUBase+r.IntN(cat.SKUs)),
			Qty:   1 + r.IntN(cat.MaxQty),
			Price: DemoCents(1 + r.Float64()*cat.MaxPrice),
		}
	}
	return out
}

// DemoCents rounds an amount to cents.
func DemoCents(v float64) float64 { return math.Round(v*100) / 100 }

// DemoMoney is the order total of items.
func DemoMoney(items []DemoOrderItem) float64 {
	total := 0.0
	for _, it := range items {
		total += it.Price * float64(it.Qty)
	}
	return DemoCents(total)
}
