package config

import "testing"

func TestRuntimeLoaderDefaultsAndOverrides(t *testing.T) {
	empty := RuntimeLoader{lookup: func(string) (string, bool) { return "", false }, hostname: func() (string, error) { return "pod-7", nil }}
	w, err := empty.Worker()
	if err != nil || w.ID != "pod-7" || w.Zone != string(DefaultWorkerZone) || w.Capacity != int(DefaultWorkerCapacity) || w.DLQTarget != string(DefaultDLQTarget) {
		t.Fatalf("defaults %+v %v", w, err)
	}
	s, err := empty.Scheduler()
	if err != nil || !s.Enabled || s.Placement != string(DefaultPlacement) {
		t.Fatalf("scheduler %+v %v", s, err)
	}
	env := map[string]string{string(EnvWorkerCapacity): "7", string(EnvSchedulerEnabled): "false", string(EnvWorkerID): "w1"}
	set := NewRuntimeLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if w, _ := set.Worker(); w.Capacity != 7 || w.ID != "w1" {
		t.Fatalf("overrides %+v", w)
	}
	if s, _ := set.Scheduler(); s.Enabled {
		t.Fatal("scheduler opt-out ignored")
	}
	env[string(EnvWorkerCapacity)] = "-1"
	if _, err := set.Worker(); err == nil {
		t.Fatal("negative capacity accepted")
	}
}
