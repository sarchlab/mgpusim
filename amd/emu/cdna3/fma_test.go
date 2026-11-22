package cdna3

import (
	"math"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

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

func TestCDNA3DivisionFMAFusesAndScales(t *testing.T) {
	tests := []struct {
		name          string
		opcode        insts.Opcode
		a, b, c       uint64
		want, wantVCC uint64
	}{
		{
			name: "v_div_fmas_f32", opcode: 482,
			a: f32bits(1 + 0x1p-23), b: f32bits(1 - 0x1p-23), c: f32bits(-1),
			want: f32bits(-0x1p-46), wantVCC: f32bits(-0x1p-14),
		},
		{
			name: "v_div_fmas_f64", opcode: 483,
			a: math.Float64bits(1 + 0x1p-52), b: math.Float64bits(1 - 0x1p-52), c: math.Float64bits(-1),
			want: math.Float64bits(-0x1p-104), wantVCC: math.Float64bits(-0x1p-40),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := newMockInstState()
			state.exec = 0b11
			state.vcc = 0b10
			state.inst.FormatType = insts.VOP3a
			state.inst.Opcode = tc.opcode
			state.inst.Src0 = &insts.Operand{}
			state.inst.Src1 = &insts.Operand{}
			state.inst.Src2 = &insts.Operand{}
			state.inst.Dst = &insts.Operand{}
			for lane := range 2 {
				state.setOperand(state.inst.Src0, lane, tc.a)
				state.setOperand(state.inst.Src1, lane, tc.b)
				state.setOperand(state.inst.Src2, lane, tc.c)
			}

			NewALU(nil).Run(state)

			if got := state.ReadOperand(state.inst.Dst, 0); got != tc.want {
				t.Errorf("lane without VCC: got %#x, want %#x", got, tc.want)
			}
			if got := state.ReadOperand(state.inst.Dst, 1); got != tc.wantVCC {
				t.Errorf("lane with VCC: got %#x, want %#x", got, tc.wantVCC)
			}
		})
	}
}
