package changes

import (
	"testing"

	"github.com/eventrouter/router/internal/runtime/domain"
)

func TestBrokerFansOutAndUnsubscribes(t *testing.T) {
	b := NewBroker()
	a, cancelA := b.Subscribe()
	c, cancelC := b.Subscribe()
	b.Publish(domain.ChangeHint{RouteID: "r1", Kind: domain.ChangeRoute})
	if (<-a).RouteID != "r1" || (<-c).RouteID != "r1" {
		t.Fatal("hint not delivered")
	}
	cancelA()
	cancelA()
	b.Publish(domain.ChangeHint{RouteID: "r2"})
	if _, ok := <-a; ok {
		t.Fatal("cancelled subscriber still receives")
	}
	if (<-c).RouteID != "r2" {
		t.Fatal("remaining subscriber lost hint")
	}
	cancelC()
}
