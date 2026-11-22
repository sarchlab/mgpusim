package bitops

import "math"

// FMAF32 rounds a*b+c once to binary32. A float64 FMA followed by a
// conversion can round twice when the float64 result lands on a float32
// midpoint even though the exact result is to one side.
func FMAF32(a, b, c float32) float32 {
	return float32(fmaRoundToOdd(a, b, c))
}

// FMAF16 evaluates a*b+c and rounds once to IEEE binary16.
func FMAF16(a, b, c float32) uint16 {
	return Float64ToFloat16(fmaRoundToOdd(a, b, c))
}

func fmaRoundToOdd(a, b, c float32) float64 {
	// The product of two finite float32 values is exact in float64,
	// including the full exponent range of products of subnormal values.
	product := float64(a) * float64(b)
	addend := float64(c)
	sum := product + addend
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return math.FMA(float64(a), float64(b), addend)
	}
	if math.Float64bits(sum)&1 != 0 {
		return sum
	}

	// Knuth's TwoSum recovers the signed error of the float64 sum. Round
	// the intermediate to odd before narrowing: no binary16 or binary32 midpoint has
	// an odd float64 significand, so the side of the midpoint is retained.
	virtualAddend := sum - product
	residual := (product - (sum - virtualAddend)) + (addend - virtualAddend)
	if residual > 0 {
		sum = math.Nextafter(sum, math.Inf(1))
	} else if residual < 0 {
		sum = math.Nextafter(sum, math.Inf(-1))
	}
	return sum
}

// Float64ToFloat16 narrows to IEEE binary16 with round-to-nearest-even.
func Float64ToFloat16(value float64) uint16 {
	raw := math.Float64bits(value)
	sign := uint16(raw >> 48 & 0x8000)
	exponent := int(raw >> 52 & 0x7ff)
	mantissa := raw & (1<<52 - 1)
	if exponent == 0x7ff && mantissa != 0 {
		return sign | 0x7e00
	}
	halfExponent := exponent - 1023 + 15
	if halfExponent >= 31 {
		return sign | 0x7c00
	}
	if halfExponent < -10 {
		return sign
	}
	shift := 42
	if halfExponent <= 0 {
		mantissa |= 1 << 52
		shift += 1 - halfExponent
		halfExponent = 0
	}
	half := uint16(halfExponent<<10) | uint16(mantissa>>shift)
	remainder := mantissa & (1<<shift - 1)
	midpoint := uint64(1) << (shift - 1)
	if remainder > midpoint || (remainder == midpoint && half&1 != 0) {
		half++
	}
	return sign | half
}
