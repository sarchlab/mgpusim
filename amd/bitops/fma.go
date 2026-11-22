package bitops

import "math"

// FMAF32 rounds a*b+c once to binary32. A float64 FMA followed by a
// conversion can round twice when the float64 result lands on a float32
// midpoint even though the exact result is to one side.
func FMAF32(a, b, c float32) float32 {
	// The product of two finite float32 values is exact in float64,
	// including the full exponent range of products of subnormal values.
	product := float64(a) * float64(b)
	addend := float64(c)
	sum := product + addend
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return float32(math.FMA(float64(a), float64(b), addend))
	}
	if math.Float64bits(sum)&1 != 0 {
		return float32(sum)
	}

	// Knuth's TwoSum recovers the signed error of the float64 sum. Round
	// the intermediate to odd before narrowing: no float32 midpoint has
	// an odd float64 significand, so the side of the midpoint is retained.
	virtualAddend := sum - product
	residual := (product - (sum - virtualAddend)) + (addend - virtualAddend)
	if residual > 0 {
		sum = math.Nextafter(sum, math.Inf(1))
	} else if residual < 0 {
		sum = math.Nextafter(sum, math.Inf(-1))
	}
	return float32(sum)
}
