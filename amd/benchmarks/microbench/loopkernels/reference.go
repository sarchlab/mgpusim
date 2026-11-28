package loopkernels

import (
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/bitops"
)

// Initial operand bit patterns, identical to common_hip/gfx942_loop_kernels.h.
// Every thread starts from the same values.
const (
	f32X0   uint32 = 0x3f800000 // 1.0f: accumulator
	f32X0Hi uint32 = 0x3f000000 // 0.5f: accumulator, upper v_pk_fma_f32 lane
	f32M    uint32 = 0x3f800001 // 1.0000001f: multiplier
	f32A    uint32 = 0x33d6bf95 // 1.0e-7f: addend

	u32X0 uint32 = 1
	u32M  uint32 = 1664525
	u32A  uint32 = 1013904223

	f64X0Lo uint32 = 0x00000000 // 1.0: accumulator
	f64X0Hi uint32 = 0x3ff00000
	f64MLo  uint32 = 0x1ad7f29b // 1.0000001: multiplier
	f64MHi  uint32 = 0x3ff00000
	f64ALo  uint32 = 0x9abcaf48 // 1.0e-7: addend
	f64AHi  uint32 = 0x3e7ad7f2

	b64Lo uint32 = 0x89abcdef // v_mov_b64: an arbitrary pattern
	b64Hi uint32 = 0x01234567
)

// VALUInstructions are the measured VALU instructions, in the order of
// LK_FOR_EACH_VALU.
var VALUInstructions = []string{
	"v_fma_f32",
	"v_mul_f32",
	"v_add_f32",
	"v_mul_lo_u32",
	"v_add_u32",
	"v_fma_f64",
	"v_pk_fma_f32",
	"v_mov_b64",
}

func pair(lo, hi uint32) uint64 {
	return uint64(hi)<<32 | uint64(lo)
}

// Reference is the 64-bit X of a VALU instruction after `applications`
// applications of X = op(X, ...) from its initial operands, a port of
// lk_reference in common_hip/gfx942_loop_host.h. GPU and CPU both round to
// nearest and fuse the FMAs, so the comparison is bit-exact. A move, and any
// name that is not a VALU instruction, leaves X at the v_mov_b64 pattern.
//
//nolint:funlen // One case per instruction, as in the HIP source.
func Reference(instruction string, applications uint64) uint64 {
	m32 := math.Float32frombits(f32M)
	a32 := math.Float32frombits(f32A)

	switch instruction {
	case "v_fma_f32":
		x := math.Float32frombits(f32X0)
		for range applications {
			x = bitops.FMAF32(x, m32, a32)
		}
		return uint64(math.Float32bits(x))
	case "v_mul_f32":
		x := math.Float32frombits(f32X0)
		for range applications {
			x *= m32
		}
		return uint64(math.Float32bits(x))
	case "v_add_f32":
		x := math.Float32frombits(f32X0)
		for range applications {
			x += a32
		}
		return uint64(math.Float32bits(x))
	case "v_mul_lo_u32":
		x := u32X0
		for range applications {
			x *= u32M
		}
		return uint64(x)
	case "v_add_u32":
		x := u32X0
		for range applications {
			x += u32A
		}
		return uint64(x)
	case "v_fma_f64":
		m := math.Float64frombits(pair(f64MLo, f64MHi))
		a := math.Float64frombits(pair(f64ALo, f64AHi))
		x := math.Float64frombits(pair(f64X0Lo, f64X0Hi))
		for range applications {
			x = math.FMA(x, m, a)
		}
		return math.Float64bits(x)
	case "v_pk_fma_f32":
		lo := math.Float32frombits(f32X0)
		hi := math.Float32frombits(f32X0Hi)
		for range applications {
			lo = bitops.FMAF32(lo, m32, a32)
			hi = bitops.FMAF32(hi, m32, a32)
		}
		return pair(math.Float32bits(lo), math.Float32bits(hi))
	}
	return pair(b64Lo, b64Hi) // a move leaves X unchanged
}
