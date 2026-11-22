package cdna3

import (
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
