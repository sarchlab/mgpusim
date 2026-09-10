// Package verification provides small helpers shared by benchmark verifiers.
package verification

import "math"

// AllFinite reports whether every value is neither NaN nor infinity.
func AllFinite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
