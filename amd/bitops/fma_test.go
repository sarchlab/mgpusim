package bitops_test

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/bitops"
)

func TestFMAF32Rounding(t *testing.T) {
	negativeZero := math.Float32frombits(0x80000000)
	tests := []struct {
		name    string
		a, b, c float32
		want    uint32
	}{
		{"cancellation", 1 + 0x1p-23, 1 - 0x1p-23, -1, 0xa8800000},
		{"below midpoint", 1 + 0x1p-23, 1.5, -0x1p-80, 0x3fc00001},
		{"above midpoint", 1 + 0x1p-12, 1 + 0x1p-12, 0x1p-100, 0x3f801001},
		{"exact cancellation", 1, 1, -1, 0},
		{"negative zero", negativeZero, 1, negativeZero, 0x80000000},
		{"halfway to subnormal", math.SmallestNonzeroFloat32, 0.5, 0, 0},
		{"halfway between subnormals", math.SmallestNonzeroFloat32, 0.5, math.SmallestNonzeroFloat32, 2},
		{"underflow to negative zero", -math.SmallestNonzeroFloat32, 0.5, 0, 0x80000000},
		{"overflow", math.MaxFloat32, 1, 0x1p103, 0x7f800000},
		{"negative infinity", float32(math.Inf(-1)), 2, 3, 0xff800000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := math.Float32bits(bitops.FMAF32(tc.a, tc.b, tc.c))
			if got != tc.want {
				t.Fatalf("got %#08x, want %#08x", got, tc.want)
			}
		})
	}
	if !math.IsNaN(float64(bitops.FMAF32(float32(math.Inf(1)), 0, 1))) {
		t.Fatal("infinity times zero must produce NaN")
	}
}

func TestFMAF32AgainstExactReference(t *testing.T) {
	random := rand.New(rand.NewPCG(0, 1))
	finite := func() float32 {
		for {
			bits := random.Uint32()
			if bits&0x7f800000 != 0x7f800000 {
				return math.Float32frombits(bits)
			}
		}
	}

	for range 3000 {
		a, b, c := finite(), finite(), finite()
		// This precision covers the entire span between the largest
		// float32 products and the smallest nonzero product bit.
		exact := new(big.Float).SetPrec(640)
		exact.Mul(big.NewFloat(float64(a)), big.NewFloat(float64(b)))
		exact.Add(exact, big.NewFloat(float64(c)))
		want, _ := exact.Float32()
		got := bitops.FMAF32(a, b, c)
		if math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("bitops.FMAF32(%x, %x, %x) = %x, want %x", a, b, c, got, want)
		}
	}
}
