package main

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestFeedConfigDefaults(t *testing.T) {
	cfg, err := FeedConfigFromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Clusters) != 2 || cfg.Clusters["b"][0] != "kafka-b:19092" {
		t.Fatalf("clusters = %v", cfg.Clusters)
	}
	if len(cfg.Streams) != 4 || cfg.Streams[0] != (StreamSpec{Generator: "orders", Cluster: "a", Topic: "demo-orders"}) {
		t.Fatalf("streams = %v", cfg.Streams)
	}
	if cfg.Interval() != time.Second || cfg.Partitions != 3 {
		t.Fatalf("interval %v partitions %d", cfg.Interval(), cfg.Partitions)
	}
}

func TestFeedConfigRejectsBadEntries(t *testing.T) {
	cases := map[string]map[string]string{
		"unknown cluster":   {"DEMO_STREAMS": "orders@c/x"},
		"unknown generator": {"DEMO_STREAMS": "nope@a/x"},
		"bad cluster":       {"DEMO_CLUSTERS": "a"},
		"zero rate":         {"DEMO_RATE": "0"},
	}
	for name, env := range cases {
		if _, err := FeedConfigFromEnv(func(k string) string { return env[k] }); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestGeneratorsProduceKeyedEvents(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	for name, gen := range Generators {
		ev := gen.Next(1000, rnd, 1)
		if ev.Key == "" || ev.Value["seq"] != int64(1000) {
			t.Errorf("%s: %+v", name, ev)
		}
	}
	if !(RawReadings{}).Next(1, rnd, 1).Value["bad"].(bool) || (RawReadings{}).Next(1, rnd, 0).Value["bad"].(bool) {
		t.Error("bad ratio not honoured")
	}
	order := (OrderEvents{}).Next(7, rnd, 0)
	if order.Key != order.Value["orderId"] || order.Value["customer"].(map[string]any)["email"] == "" {
		t.Errorf("order = %+v", order)
	}
}
