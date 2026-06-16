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
	state.inst.Opcode = 929
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
