package verification

import (
	"math"
	"testing"
)

func TestAllFinite(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   bool
	}{
		{name: "empty", want: true},
		{name: "finite", values: []float64{0, math.SmallestNonzeroFloat64, math.MaxFloat64}, want: true},
		{name: "NaN", values: []float64{1, math.NaN()}, want: false},
		{name: "positive infinity", values: []float64{1, math.Inf(1)}, want: false},
		{name: "negative infinity", values: []float64{math.Inf(-1), 1}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := AllFinite(test.values...); got != test.want {
				t.Errorf("AllFinite(%v) = %t, want %t", test.values, got, test.want)
			}
		})
	}
}
