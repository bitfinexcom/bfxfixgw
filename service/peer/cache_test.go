package peer

import (
	"fmt"
	"testing"
)

func TestAvgFillPx(t *testing.T) {
	cache := CachedOrder{
		Executions: make([]execution, 0),
	}
	// (1600 * 0.1 + 1650 * 0.5 + 1675 * 1.2) / (0.1 + 0.5 + 1.2) = 1663.888889
	cache.Executions = append(cache.Executions, execution{Px: 1600, Qty: 0.1})
	cache.Executions = append(cache.Executions, execution{Px: 1650, Qty: 0.5})
	cache.Executions = append(cache.Executions, execution{Px: 1675, Qty: 1.2})
	avg := cache.AvgFillPx()
	str := fmt.Sprintf("%0.2f", avg) // round to compare
	if "1663.89" != str {
		t.Fatalf("expected 1663.888889, got %f", avg)
	}
}
