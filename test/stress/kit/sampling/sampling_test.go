package sampling

import "testing"

const metricsText = `# HELP router_events_consumed_total x
# TYPE router_events_consumed_total counter
router_events_consumed_total{branch="main",route_id="r1",worker="w"} 100
router_events_consumed_total{branch="main",route_id="r2",worker="w"} 50
router_events_consumed_total{branch="main",route_id="other",worker="w"} 7
router_consumer_lag{partition="0",route_id="r1"} 3
router_consumer_lag{partition="1",route_id="r1"} 4
go_goroutines 45
process_resident_memory_bytes 6.483968e+07
weird{msg="a \"quoted\", value",route_id="r1"} 1
`

func TestPromTextSelect(t *testing.T) {
	series := PromText{}.Parse([]byte(metricsText))
	routes := map[string]bool{"r1": true, "r2": true}
	if v, _ := (Select{Name: "router_events_consumed_total", Label: "route_id", Values: routes}).Sum(series); v != 150 {
		t.Errorf("consumed = %v", v)
	}
	if v, _ := (Select{Name: "router_consumer_lag", Label: "route_id", Values: routes}).Sum(series); v != 7 {
		t.Errorf("lag = %v", v)
	}
	if v, ok := (Select{Name: "process_resident_memory_bytes"}).Sum(series); !ok || v != 64839680 {
		t.Errorf("rss = %v %v", v, ok)
	}
	if _, ok := (Select{Name: "absent"}).Sum(series); ok {
		t.Error("absent metric matched")
	}
	for _, s := range series {
		if s.Name == "weird" && (s.Labels["route_id"] != "r1" || s.Labels["msg"] != `a "quoted", value`) {
			t.Errorf("escaped labels = %v", s.Labels)
		}
	}
}

func TestProcStatParse(t *testing.T) {
	raw := []byte("1234 (my (odd) worker) S 1 1234 1234 0 -1 4194560 1000 0 0 0 250 50 0 0 20 0 17 0 100 900000000 2048 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0")
	ps := ProcStatParser{}.Fill(ProcessSnapshot{Name: "w"}, raw)
	if ps.Err != "" || ps.CPUSeconds != 3 || ps.Threads != 17 || ps.RSSBytes != 2048*4096 {
		t.Errorf("stat = %+v", ps)
	}
	if bad := (ProcStatParser{}).Fill(ProcessSnapshot{}, []byte("1 (x) S 1")); bad.Err == "" {
		t.Error("short stat accepted")
	}
}

func TestPIDResolver(t *testing.T) {
	if pid, err := (PIDResolver{}).Parse([]byte(" 4242\n")); err != nil || pid != 4242 {
		t.Errorf("pid = %d %v", pid, err)
	}
	if _, err := (PIDResolver{}).Parse(nil); err == nil {
		t.Error("empty pid accepted")
	}
}
