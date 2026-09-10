package cdna3

import (
	"math"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func f32bits(f float32) uint64 { return uint64(math.Float32bits(f)) }

// TestVOPCNgtF32 checks v_cmp_ngt_f32 (0x4b): result = !(s0 > s1),
// which is also true when either operand is NaN.
func TestVOPCNgtF32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOPC
	state.inst.Opcode = 0x4b
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.exec = 0xF // lanes 0-3

	// lane0: 3>2 -> ngt false
	state.setOperand(state.inst.Src0, 0, f32bits(3))
	state.setOperand(state.inst.Src1, 0, f32bits(2))
	// lane1: 1>2 false -> ngt true
	state.setOperand(state.inst.Src0, 1, f32bits(1))
	state.setOperand(state.inst.Src1, 1, f32bits(2))
	// lane2: 2>2 false -> ngt true
	state.setOperand(state.inst.Src0, 2, f32bits(2))
	state.setOperand(state.inst.Src1, 2, f32bits(2))
	// lane3: NaN -> ngt true
	state.setOperand(state.inst.Src0, 3, f32bits(float32(math.NaN())))
	state.setOperand(state.inst.Src1, 3, f32bits(2))

	alu.Run(state)

	want := uint64(0b1110)
	if state.vcc != want {
		t.Fatalf("v_cmp_ngt_f32 expected VCC=%b, got %b", want, state.vcc)
	}
}

func TestVOPCNltF32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOPC
	state.inst.Opcode = 0x4e
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.exec = 0x7

	state.setOperand(state.inst.Src0, 0, f32bits(1)) // 1<2 -> nlt false
	state.setOperand(state.inst.Src1, 0, f32bits(2))
	state.setOperand(state.inst.Src0, 1, f32bits(3)) // 3<2 false -> nlt true
	state.setOperand(state.inst.Src1, 1, f32bits(2))
	state.setOperand(state.inst.Src0, 2, f32bits(float32(math.NaN())))
	state.setOperand(state.inst.Src1, 2, f32bits(2)) // NaN -> nlt true

	alu.Run(state)
	want := uint64(0b110)
	if state.vcc != want {
		t.Fatalf("v_cmp_nlt_f32 expected VCC=%b, got %b", want, state.vcc)
	}
}

func TestVOP3aAddF32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 257
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1

	state.setOperand(state.inst.Src0, 0, f32bits(1.5))
	state.setOperand(state.inst.Src1, 0, f32bits(2.25))
	alu.Run(state)

	got := math.Float32frombits(uint32(state.operands[state.inst.Dst][0]))
	if got != 3.75 {
		t.Fatalf("v_add_f32 expected 3.75, got %v", got)
	}
}

func TestVOP3aMadI64I32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 489
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Src2 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1

	// (-3) * 7 + 100 = 79
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(-3)))
	state.setOperand(state.inst.Src1, 0, uint64(emu.Int32ToBits(7)))
	state.setOperand(state.inst.Src2, 0, emu.Int64ToBits(100))
	alu.Run(state)

	got := emu.AsInt64(state.operands[state.inst.Dst][0])
	if got != 79 {
		t.Fatalf("v_mad_i64_i32 expected 79, got %d", got)
	}

	// Test 64-bit result that exceeds 32 bits: 0x40000000 * 4 = 0x100000000
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(0x40000000)))
	state.setOperand(state.inst.Src1, 0, uint64(emu.Int32ToBits(4)))
	state.setOperand(state.inst.Src2, 0, emu.Int64ToBits(0))
	alu.Run(state)
	got = emu.AsInt64(state.operands[state.inst.Dst][0])
	if got != 0x100000000 {
		t.Fatalf("v_mad_i64_i32 expected 0x100000000, got 0x%x", got)
	}
}

func TestVOP3aOr3B32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 514
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Src2 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1

	state.setOperand(state.inst.Src0, 0, 0x0000000F)
	state.setOperand(state.inst.Src1, 0, 0x00000F00)
	state.setOperand(state.inst.Src2, 0, 0x0F000000)
	alu.Run(state)

	if state.operands[state.inst.Dst][0] != 0x0F000F0F {
		t.Fatalf("v_or3_b32 expected 0x0F000F0F, got 0x%08x", state.operands[state.inst.Dst][0])
	}
}

func TestVOP3aLdexpF32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 648
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x3

	state.setOperand(state.inst.Src0, 0, f32bits(1.5))
	state.setOperand(state.inst.Src1, 0, 3) // 1.5 * 2^3 = 12
	state.setOperand(state.inst.Src0, 1, f32bits(8.0))
	state.setOperand(state.inst.Src1, 1, uint64(emu.Int32ToBits(-2))) // 8 * 2^-2 = 2
	alu.Run(state)

	g0 := math.Float32frombits(uint32(state.operands[state.inst.Dst][0]))
	g1 := math.Float32frombits(uint32(state.operands[state.inst.Dst][1]))
	if g0 != 12.0 {
		t.Fatalf("v_ldexp_f32 lane0 expected 12, got %v", g0)
	}
	if g1 != 2.0 {
		t.Fatalf("v_ldexp_f32 lane1 expected 2, got %v", g1)
	}
}

func TestVOP3aSubU32Clamp(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 309
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x3

	// no clamp: 3 - 5 wraps
	state.inst.Clamp = false
	state.setOperand(state.inst.Src0, 0, 3)
	state.setOperand(state.inst.Src1, 0, 5)
	state.setOperand(state.inst.Src0, 1, 10)
	state.setOperand(state.inst.Src1, 1, 4)
	alu.Run(state)
	var a, b uint32 = 3, 5
	if uint32(state.operands[state.inst.Dst][0]) != a-b {
		t.Fatalf("v_sub_u32 no-clamp lane0 wrong: 0x%x", state.operands[state.inst.Dst][0])
	}
	if state.operands[state.inst.Dst][1] != 6 {
		t.Fatalf("v_sub_u32 lane1 expected 6, got %d", state.operands[state.inst.Dst][1])
	}

	// clamp: 3 - 5 saturates to 0
	state.inst.Clamp = true
	alu.Run(state)
	if state.operands[state.inst.Dst][0] != 0 {
		t.Fatalf("v_sub_u32 clamp lane0 expected 0, got %d", state.operands[state.inst.Dst][0])
	}
	if state.operands[state.inst.Dst][1] != 6 {
		t.Fatalf("v_sub_u32 clamp lane1 expected 6, got %d", state.operands[state.inst.Dst][1])
	}
}

func TestSOP2MulHiI32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP2
	state.inst.Opcode = 45
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	// (-1) * (-1) = 1, high word = 0
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(-1)))
	state.setOperand(state.inst.Src1, 0, uint64(emu.Int32ToBits(-1)))
	alu.Run(state)
	if state.operands[state.inst.Dst][0] != 0 {
		t.Fatalf("s_mul_hi_i32 (-1*-1) high expected 0, got 0x%x", state.operands[state.inst.Dst][0])
	}

	// 0x40000000 * 0x40000000 = 0x1000000000000000, high word = 0x10000000
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(0x40000000)))
	state.setOperand(state.inst.Src1, 0, uint64(emu.Int32ToBits(0x40000000)))
	alu.Run(state)
	if uint32(state.operands[state.inst.Dst][0]) != 0x10000000 {
		t.Fatalf("s_mul_hi_i32 expected 0x10000000, got 0x%x", state.operands[state.inst.Dst][0])
	}

	// (-2) * 0x40000000 = -0x80000000 = 0xFFFFFFFF80000000 (i64); >>32 = -1 = 0xFFFFFFFF
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(-2)))
	state.setOperand(state.inst.Src1, 0, uint64(emu.Int32ToBits(0x40000000)))
	alu.Run(state)
	if uint32(state.operands[state.inst.Dst][0]) != 0xFFFFFFFF {
		t.Fatalf("s_mul_hi_i32 signed expected 0xFFFFFFFF, got 0x%x", state.operands[state.inst.Dst][0])
	}
}

func TestVOP3aPkAddF16(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 911
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1
	// A plain packed add decodes op_sel = 0b00, op_sel_hi = 0b11, so the high
	// result word reads the high f16 halves: hi = s0.hi + s1.hi.
	state.inst.OpSelHi = 3

	// src0 = {lo=1.0, hi=2.0}, src1 = {lo=0.5, hi=3.0}
	src0 := uint64(float32ToFloat16(1.0)) | (uint64(float32ToFloat16(2.0)) << 16)
	src1 := uint64(float32ToFloat16(0.5)) | (uint64(float32ToFloat16(3.0)) << 16)
	state.setOperand(state.inst.Src0, 0, src0)
	state.setOperand(state.inst.Src1, 0, src1)
	alu.Run(state)

	res := uint32(state.operands[state.inst.Dst][0])
	lo := float16ToFloat32(uint16(res & 0xFFFF))
	hi := float16ToFloat32(uint16(res >> 16))
	if lo != 1.5 {
		t.Fatalf("v_pk_add_f16 lo expected 1.5, got %v", lo)
	}
	if hi != 5.0 {
		t.Fatalf("v_pk_add_f16 hi expected 5.0, got %v", hi)
	}
}

func TestVOP3aPkAddF16DecodedModifiersAndEXEC(t *testing.T) {
	testCases := []struct {
		name string
		code []byte
		src0 uint64
		src1 uint64
		want uint32
	}{
		{
			name: "non-symmetric decoded high-half selection",
			// v_pk_add_f16 with OPSEL_HI src0=1, src1=0. Bit 14 is set,
			// while the unrelated src2 selector at bit 59 is clear.
			code: []byte{0x01, 0x40, 0x8f, 0xd3, 0x01, 0x05, 0x01, 0x00},
			src0: packF16Pair(1.0, 2.0),
			src1: packF16Pair(0.5, 3.0),
			want: 0x41003e00, // {1.5, 2.5}
		},
		{
			name: "independent low and high negation",
			// v_pk_add_f16 v1, v1, v2
			//     neg_lo:[1,0] neg_hi:[0,1]
			code: []byte{0x01, 0x42, 0x8f, 0xd3, 0x01, 0x05, 0x02, 0x38},
			src0: packF16Pair(1.0, 2.0),
			src1: packF16Pair(0.5, 3.0),
			want: 0xbc00b800, // {-0.5, -1.0}
		},
		{
			name: "clamp applies its lower bound to both halves",
			// Same instruction with CLMP set.
			code: []byte{0x01, 0xc2, 0x8f, 0xd3, 0x01, 0x05, 0x02, 0x38},
			src0: packF16Pair(1.0, 2.0),
			src1: packF16Pair(0.5, 3.0),
			want: 0x00000000,
		},
		{
			name: "clamp applies its upper bound to both halves",
			code: []byte{0x01, 0xc2, 0x8f, 0xd3, 0x01, 0x05, 0x02, 0x38},
			src0: packF16Pair(-2.0, 2.0),
			src1: packF16Pair(0.5, -3.0),
			want: 0x3c003c00,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst, err := insts.NewDisassembler().Decode(tc.code)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			alu := NewALU(nil)
			state := newMockInstState()
			state.inst = inst
			state.exec = 0x1

			state.setOperand(inst.Src0, 0, tc.src0)
			state.setOperand(inst.Src1, 0, tc.src1)
			state.setOperand(inst.Src0, 1, tc.src0)
			state.setOperand(inst.Src1, 1, tc.src1)
			const disabledLaneSentinel = uint64(0x5aa5c33c)
			state.setOperand(inst.Dst, 1, disabledLaneSentinel)

			alu.Run(state)

			if got := uint32(state.operands[inst.Dst][0]); got != tc.want {
				t.Fatalf("result = 0x%08x, want 0x%08x", got, tc.want)
			}
			if got := state.operands[inst.Dst][1]; got != disabledLaneSentinel {
				t.Fatalf("disabled EXEC lane changed to 0x%08x", got)
			}
		})
	}
}

func TestVOP3aPackB32F16(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 672
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1

	state.setOperand(state.inst.Src0, 0, 0xabcd1234)
	state.setOperand(state.inst.Src1, 0, 0xef015678)
	alu.Run(state)
	if got := uint32(state.operands[state.inst.Dst][0]); got != 0x56781234 {
		t.Fatalf("v_pack_b32_f16 low selection = 0x%08x, want 0x56781234", got)
	}

	state.inst.OpSel = 0b11
	alu.Run(state)
	if got := uint32(state.operands[state.inst.Dst][0]); got != 0xef01abcd {
		t.Fatalf("v_pack_b32_f16 high selection = 0x%08x, want 0xef01abcd", got)
	}
}

func TestVOP3aPackB32F16DecodedModifiersAndEXEC(t *testing.T) {
	// v_pack_b32_f16 v2, -|v2|, |v3| op_sel:[1,0]
	code := []byte{0x02, 0x0b, 0xa0, 0xd2, 0x02, 0x07, 0x02, 0x20}
	inst, err := insts.NewDisassembler().Decode(code)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	alu := NewALU(nil)
	state := newMockInstState()
	state.inst = inst
	state.exec = 0x1

	// OPSEL selects src0.high (+3) and src1.low (-4). Source modifiers
	// produce -abs(+3) and abs(-4), preserving their f16 encodings.
	src0 := uint64(float32ToFloat16(7.0)) |
		(uint64(float32ToFloat16(3.0)) << 16)
	src1 := uint64(float32ToFloat16(-4.0)) |
		(uint64(float32ToFloat16(-8.0)) << 16)
	state.setOperand(inst.Src0, 0, src0)
	state.setOperand(inst.Src1, 0, src1)
	state.setOperand(inst.Src0, 1, src0)
	state.setOperand(inst.Src1, 1, src1)
	const disabledLaneSentinel = uint64(0x5aa5c33c)
	state.setOperand(inst.Dst, 1, disabledLaneSentinel)

	alu.Run(state)

	if got := uint32(state.operands[inst.Dst][0]); got != 0x4400c200 {
		t.Fatalf("result = 0x%08x, want 0x4400c200", got)
	}
	if got := state.operands[inst.Dst][1]; got != disabledLaneSentinel {
		t.Fatalf("disabled EXEC lane changed to 0x%08x", got)
	}
}

func TestVOP3aPackB32F16DecodedClamp(t *testing.T) {
	// v_pack_b32_f16 v2, v2, v3 clamp
	code := []byte{0x02, 0x80, 0xa0, 0xd2, 0x02, 0x07, 0x02, 0x00}
	inst, err := insts.NewDisassembler().Decode(code)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	alu := NewALU(nil)
	state := newMockInstState()
	state.inst = inst
	state.exec = 0x1
	state.setOperand(inst.Src0, 0, packF16Pair(-2, 7))
	state.setOperand(inst.Src1, 0, packF16Pair(3, 8))

	alu.Run(state)

	if got := uint32(state.operands[inst.Dst][0]); got != 0x3c000000 {
		t.Fatalf("clamped result = 0x%08x, want 0x3c000000", got)
	}
}

func TestVOP3aFMAMixF16PreservesOtherHalf(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 929
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Src2 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x1

	// The raw fp16-throughput instructions use f32 sources (OpSelHi == 0).
	// MIXLO computes 1.5*2+0.25 = 3.25 and keeps the existing high half.
	state.setOperand(state.inst.Src0, 0, f32bits(1.5))
	state.setOperand(state.inst.Src1, 0, f32bits(2))
	state.setOperand(state.inst.Src2, 0, f32bits(0.25))
	state.setOperand(state.inst.Dst, 0,
		uint64(uint32(float32ToFloat16(9))<<16))
	alu.Run(state)

	got := uint32(state.operands[state.inst.Dst][0])
	if lo := float16ToFloat32(uint16(got)); lo != 3.25 {
		t.Fatalf("v_fma_mixlo_f16 low result = %v, want 3.25", lo)
	}
	if hi := float16ToFloat32(uint16(got >> 16)); hi != 9 {
		t.Fatalf("v_fma_mixlo_f16 changed high half to %v, want 9", hi)
	}

	// MIXHI computes 2*3+1 = 7 and keeps the low result from MIXLO.
	state.inst.Opcode = 930
	state.setOperand(state.inst.Src0, 0, f32bits(2))
	state.setOperand(state.inst.Src1, 0, f32bits(3))
	state.setOperand(state.inst.Src2, 0, f32bits(1))
	alu.Run(state)

	got = uint32(state.operands[state.inst.Dst][0])
	if lo := float16ToFloat32(uint16(got)); lo != 3.25 {
		t.Fatalf("v_fma_mixhi_f16 changed low half to %v, want 3.25", lo)
	}
	if hi := float16ToFloat32(uint16(got >> 16)); hi != 7 {
		t.Fatalf("v_fma_mixhi_f16 high result = %v, want 7", hi)
	}
}

func TestVOP3aFMAMixF16ModifiersClampAndEXEC(t *testing.T) {
	t.Run("decoded non-symmetric op_sel_hi selects src0 and src1 as f16", func(t *testing.T) {
		// v_fma_mixlo_f16 with OPSEL_HI src0=1, src1=1, src2=0.
		code := []byte{0x01, 0x40, 0xa1, 0xd3, 0x02, 0x09, 0x0c, 0x14}
		inst, err := insts.NewDisassembler().Decode(code)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		alu := NewALU(nil)
		state := newMockInstState()
		state.inst = inst
		state.exec = 0x1
		state.setOperand(inst.Src0, 0, packF16Pair(2, 9))
		state.setOperand(inst.Src1, 0, packF16Pair(3, 8))
		state.setOperand(inst.Src2, 0, f32bits(1))
		state.setOperand(inst.Dst, 0, packF16Pair(4, 11))

		alu.Run(state)

		got := uint32(state.operands[inst.Dst][0])
		if low := float16ToFloat32(uint16(got)); low != 7 {
			t.Fatalf("low result = %v, want 7", low)
		}
		if high := float16ToFloat32(uint16(got >> 16)); high != 11 {
			t.Fatalf("mixlo changed high half to %v, want 11", high)
		}
	})

	t.Run("mixlo selects f16 halves and applies abs and neg", func(t *testing.T) {
		alu := NewALU(nil)
		state := newMockInstState()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 929
		state.inst.InstName = "v_fma_mixlo_f16"
		state.inst.Src0 = &insts.Operand{}
		state.inst.Src1 = &insts.Operand{}
		state.inst.Src2 = &insts.Operand{}
		state.inst.Dst = &insts.Operand{}
		state.inst.OpSelHi = 0b111 // all three inputs are selected as f16
		state.inst.OpSel = 0b010   // src0.low, src1.high, src2.low
		state.inst.Abs = 0b011     // abs(src0), abs(src1)
		state.inst.Neg = 0b110     // negate src1 and src2 after abs
		state.exec = 0x1

		state.setOperand(state.inst.Src0, 0, packF16Pair(-2, 9))
		state.setOperand(state.inst.Src1, 0, packF16Pair(8, -3))
		state.setOperand(state.inst.Src2, 0, packF16Pair(-0.5, 7))
		state.setOperand(state.inst.Dst, 0, packF16Pair(4, 11))
		const disabledLaneSentinel = uint64(0xa55a3cc3)
		state.setOperand(state.inst.Dst, 1, disabledLaneSentinel)

		alu.Run(state)

		got := uint32(state.operands[state.inst.Dst][0])
		if low := float16ToFloat32(uint16(got)); low != -5.5 {
			t.Fatalf("low result = %v, want -5.5", low)
		}
		if high := float16ToFloat32(uint16(got >> 16)); high != 11 {
			t.Fatalf("mixlo changed high half to %v, want 11", high)
		}
		if got := state.operands[state.inst.Dst][1]; got != disabledLaneSentinel {
			t.Fatalf("disabled EXEC lane changed to 0x%08x", got)
		}
	})

	t.Run("mixhi selects f16 halves and clamps", func(t *testing.T) {
		alu := NewALU(nil)
		state := newMockInstState()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 930
		state.inst.InstName = "v_fma_mixhi_f16"
		state.inst.Src0 = &insts.Operand{}
		state.inst.Src1 = &insts.Operand{}
		state.inst.Src2 = &insts.Operand{}
		state.inst.Dst = &insts.Operand{}
		state.inst.OpSelHi = 0b111 // all three inputs are selected as f16
		state.inst.OpSel = 0b101   // src0.high, src1.low, src2.high
		state.inst.Abs = 0b111
		state.inst.Neg = 0b100 // negate src2 after abs
		state.inst.Clamp = true
		state.exec = 0x1

		state.setOperand(state.inst.Src0, 0, packF16Pair(9, -2))
		state.setOperand(state.inst.Src1, 0, packF16Pair(-3, 8))
		state.setOperand(state.inst.Src2, 0, packF16Pair(7, -0.5))
		state.setOperand(state.inst.Dst, 0, packF16Pair(4, 11))
		const disabledLaneSentinel = uint64(0xa55a3cc3)
		state.setOperand(state.inst.Dst, 1, disabledLaneSentinel)

		alu.Run(state)

		got := uint32(state.operands[state.inst.Dst][0])
		if low := float16ToFloat32(uint16(got)); low != 4 {
			t.Fatalf("mixhi changed low half to %v, want 4", low)
		}
		if high := float16ToFloat32(uint16(got >> 16)); high != 1 {
			t.Fatalf("clamped high result = %v, want 1", high)
		}
		if got := state.operands[state.inst.Dst][1]; got != disabledLaneSentinel {
			t.Fatalf("disabled EXEC lane changed to 0x%08x", got)
		}
	})
}

func packF16Pair(low, high float32) uint64 {
	return uint64(float32ToFloat16(low)) |
		(uint64(float32ToFloat16(high)) << 16)
}

func TestVOP3aXadU32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 499
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Src2 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x5

	// Lane 0 wraps in uint32 arithmetic. Lane 1 is disabled and must retain
	// its destination value.
	state.setOperand(state.inst.Src0, 0, 0xffffffff)
	state.setOperand(state.inst.Src1, 0, 0)
	state.setOperand(state.inst.Src2, 0, 1)
	state.setOperand(state.inst.Src0, 1, 0x00ff00ff)
	state.setOperand(state.inst.Src1, 1, 0x0f0f0f0f)
	state.setOperand(state.inst.Src2, 1, 7)
	state.setOperand(state.inst.Dst, 1, 0x12345678)
	state.setOperand(state.inst.Src0, 2, 0x00ff00ff)
	state.setOperand(state.inst.Src1, 2, 0x0f0f0f0f)
	state.setOperand(state.inst.Src2, 2, 7)
	alu.Run(state)

	if got := uint32(state.operands[state.inst.Dst][0]); got != 0 {
		t.Fatalf("v_xad_u32 wrapped result = 0x%08x, want 0", got)
	}
	if got := uint32(state.operands[state.inst.Dst][1]); got != 0x12345678 {
		t.Fatalf("v_xad_u32 changed disabled lane to 0x%08x", got)
	}
	want := (uint32(0x00ff00ff) ^ uint32(0x0f0f0f0f)) + 7
	if got := uint32(state.operands[state.inst.Dst][2]); got != want {
		t.Fatalf("v_xad_u32 nontrivial result = 0x%08x, want 0x%08x", got, want)
	}
}

func TestFloat16ToFloat32Subnormal(t *testing.T) {
	// Half subnormals (exp field 0) must decode with the correct exponent.
	// 0x0200 (mant 0x200) is 2^-15, and 0x0001 is the smallest positive
	// subnormal 2^-24.
	if got := float16ToFloat32(0x0200); got != float32(1.0)/32768 {
		t.Fatalf("float16ToFloat32(0x0200) = %v, want 2^-15", got)
	}
	if got := float16ToFloat32(0x0001); got != float32(1.0)/16777216 {
		t.Fatalf("float16ToFloat32(0x0001) = %v, want 2^-24", got)
	}
}

func TestVOP3aNgtF32E64(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 75 // v_cmp_ngt_f32_e64
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}
	state.exec = 0x3

	state.setOperand(state.inst.Src0, 0, f32bits(3)) // 3>2 -> ngt false
	state.setOperand(state.inst.Src1, 0, f32bits(2))
	state.setOperand(state.inst.Src0, 1, f32bits(1)) // 1>2 false -> ngt true
	state.setOperand(state.inst.Src1, 1, f32bits(2))
	alu.Run(state)

	if state.operands[state.inst.Dst][0] != 0b10 {
		t.Fatalf("v_cmp_ngt_f32_e64 expected SGPR=%b, got %b", 0b10, state.operands[state.inst.Dst][0])
	}
}

// TestSOPKCmpK checks the s_cmpk_* compare-with-constant family (opcodes 4-13):
// each sets SCC from comparing Dst against the sign-extended SImm16. These are
// emitted by parameterized kernels (loop bounds against a constant) but not by
// the older frozen-constant kernels.
func TestSOPKCmpK(t *testing.T) {
	cases := []struct {
		name   string
		opcode insts.Opcode
		src    int32
		imm    int16
		want   byte
	}{
		{"gt_i32 11>10", 4, 11, 10, 1},
		{"gt_i32 10>10", 4, 10, 10, 0},
		{"ge_i32 10>=10", 5, 10, 10, 1},
		{"ge_i32 9>=10", 5, 9, 10, 0},
		{"lt_i32 5<10", 6, 5, 10, 1},
		{"lt_i32 10<10", 6, 10, 10, 0},
		{"lt_i32 -3<2", 6, -3, 2, 1},
		{"le_i32 10<=10", 7, 10, 10, 1},
		{"le_i32 11<=10", 7, 11, 10, 0},
		{"eq_u32 7==7", 8, 7, 7, 1},
		{"lg_u32 7!=8", 9, 7, 8, 1},
		{"gt_u32 9>8", 10, 9, 8, 1},
		{"ge_u32 8>=9", 11, 8, 9, 0},
		{"lt_u32 8<9", 12, 8, 9, 1},
		{"le_u32 9<=9", 13, 9, 9, 1},
	}
	for _, c := range cases {
		alu := NewALU(nil)
		state := newMockInstState()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = c.opcode
		state.inst.Dst = &insts.Operand{}
		state.inst.SImm16 = &insts.Operand{}
		state.setOperand(state.inst.Dst, 0, uint64(emu.Int32ToBits(c.src)))
		state.setOperand(state.inst.SImm16, 0, uint64(uint16(c.imm)))
		alu.Run(state)
		if state.scc != c.want {
			t.Fatalf("%s (opcode %d): expected SCC=%d, got %d",
				c.name, c.opcode, c.want, state.scc)
		}
	}
}
