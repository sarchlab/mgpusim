package cdna3

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
			got := math.Float32bits(fmaF32(tc.a, tc.b, tc.c))
			if got != tc.want {
				t.Fatalf("got %#08x, want %#08x", got, tc.want)
			}
		})
	}
	if !math.IsNaN(float64(fmaF32(float32(math.Inf(1)), 0, 1))) {
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
		got := fmaF32(a, b, c)
		if math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("fmaF32(%x, %x, %x) = %x, want %x", a, b, c, got, want)
		}
	}
}

func TestCDNA3ScalarFMAF32Opcodes(t *testing.T) {
	a, b, c := f32bits(1+0x1p-23), f32bits(1-0x1p-23), f32bits(-1)
	tests := []struct {
		name     string
		format   insts.FormatType
		opcode   insts.Opcode
		src1     uint64
		src2     uint64
		initialD uint64
	}{
		{"v_fmamk_f32", insts.VOP2, 23, c, b, 0},
		{"v_fmaak_f32", insts.VOP2, 24, b, c, 0},
		{"v_fmac_f32", insts.VOP2, 59, b, 0, c},
		{"v_fma_f32", insts.VOP3a, 459, b, c, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := newMockInstState()
			state.exec = 1
			state.inst.FormatType = tc.format
			state.inst.Opcode = tc.opcode
			state.inst.Src0 = &insts.Operand{}
			state.inst.Src1 = &insts.Operand{}
			state.inst.Src2 = &insts.Operand{}
			state.inst.Dst = &insts.Operand{}
			state.setOperand(state.inst.Src0, 0, a)
			state.setOperand(state.inst.Src1, 0, tc.src1)
			state.setOperand(state.inst.Src2, 0, tc.src2)
			state.setOperand(state.inst.Dst, 0, tc.initialD)

			NewALU(nil).Run(state)

			if got := state.ReadOperand(state.inst.Dst, 0); got != 0xa8800000 {
				t.Fatalf("got %#08x, want %#08x", got, uint64(0xa8800000))
			}
		})
	}
}

func TestCDNA3PackedFMAF32RoundsBothHalves(t *testing.T) {
	pack := func(lo, hi float32) uint64 {
		return f32bits(lo) | f32bits(hi)<<32
	}
	state := newMockInstState()
	state.exec = 1
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 944
	state.inst.OpSelHi = 7
	state.inst.Src0 = &insts.Operand{OperandType: insts.RegOperand}
	state.inst.Src1 = &insts.Operand{OperandType: insts.RegOperand}
	state.inst.Src2 = &insts.Operand{OperandType: insts.RegOperand}
	state.inst.Dst = &insts.Operand{}
	state.setOperand(state.inst.Src0, 0, pack(1+0x1p-23, 1+0x1p-23))
	state.setOperand(state.inst.Src1, 0, pack(1-0x1p-23, 1.5))
	state.setOperand(state.inst.Src2, 0, pack(-1, -0x1p-80))

	NewALU(nil).Run(state)

	const want = uint64(0x3fc00001a8800000)
	if got := state.ReadOperand(state.inst.Dst, 0); got != want {
		t.Fatalf("got %#016x, want %#016x", got, want)
	}
}
