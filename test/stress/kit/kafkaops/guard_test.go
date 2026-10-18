package kafkaops

import "testing"

func TestGuardRefusesForeignNames(t *testing.T) {
	if err := StressGuard.Check("stress-a", "stress-b-dlq"); err != nil {
		t.Errorf("stress names refused: %v", err)
	}
	for _, name := range []string{"orders", "router.dlq.stress-x", "acc-stress-1", ""} {
		if StressGuard.Check("stress-ok", name) == nil {
			t.Errorf("%q accepted", name)
		}
	}
}
