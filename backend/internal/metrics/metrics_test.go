package metrics

import "testing"

func TestMetrics_IncAndAvg(t *testing.T) {
	RequestsTotal = 0
	IncRequests()
	IncRequests()
	if RequestsTotal != 2 { t.Fatalf("expected 2, got %d", RequestsTotal) }
	latencySum = 0
	latencyCount = 0
	ObserveLatency(1000000 * 1000) // 1s in ns
	if AvgLatency() == 0 { t.Fatal("avg should not be 0") }
}
