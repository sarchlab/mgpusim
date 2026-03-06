package emu

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// helper to write a uint32 value to a specific lane and vreg in the vRegFile
func writeVRegU32(state *mockInstState, lane, reg int, val uint32) {
	offset := lane*256*4 + reg*4
	copy(state.vRegFile[offset:], insts.Uint32ToBytes(val))
}

// helper to write a uint64 value to a specific lane and vreg pair in the vRegFile
func writeVRegU64(state *mockInstState, lane, reg int, val uint64) {
	offset := lane*256*4 + reg*4
	copy(state.vRegFile[offset:], insts.Uint64ToBytes(val))
}

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALU(nil)

		state = newMockInstState()
	})

	It("should run v_cmp_lt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x41
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_eq_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x42
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(-2.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_le_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x43
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(-2.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_gt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x44
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_lg_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x45
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_ge_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x46
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_nge_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x49
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_nlg_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4A
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_ngt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4B
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_nle_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4C
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_neq_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4D
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_nlt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4E
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(-1.2))
		writeVRegU32(state, 0, 1, math.Float32bits(-1.2))
		writeVRegU32(state, 1, 0, math.Float32bits(-2.5))
		writeVRegU32(state, 1, 1, math.Float32bits(0.0))
		writeVRegU32(state, 2, 0, math.Float32bits(1.5))
		writeVRegU32(state, 2, 1, math.Float32bits(0.0))

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_lt_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x8)))
	})

	It("should run v_cmp_le_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC3
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x9)))
	})

	It("should run v_cmp_gt_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC4
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_lg_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC5
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0xE)))
	})

	It("should run v_cmp_ge_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC6
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x7)))
	})

	It("should run v_cmp_lt_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC9
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xF

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, int32ToBits(-1))
		writeVRegU32(state, 1, 1, int32ToBits(-2))
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)
		writeVRegU32(state, 3, 0, 1)
		writeVRegU32(state, 3, 1, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x8)))
	})

	It("should run v_cmp_eq_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCA
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_le_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCB
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xffffffffffffffff

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		// lanes 0..63 all active. Lanes 0,1 have src0<=src1, lane 2 has src0>src1.
		// Lanes 3..63 both src0 and src1 are 0, so 0<=0 => true.
		// Result: all bits set except bit 2 => 0xfffffffffffffffb
		Expect(state.vcc).To(Equal(uint64(0xfffffffffffffffb)))
	})

	It("should run v_cmp_gt_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCC
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_ne_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCD
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0xffffffffffffffff

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 0)
		writeVRegU32(state, 1, 1, 2)

		alu.Run(state)

		// lane 0: 1==1, not ne => 0
		// lane 1: 0!=2 => 1
		// lanes 2..63: 0==0 => 0
		Expect(state.vcc).To(Equal(uint64(0x0000000000000002)))
	})

	It("should run v_cmp_ge_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCE
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_f_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xE8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x1

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x0)))
	})

	It("should run v_cmp_lt_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xE9
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x3

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_eq_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEA
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_le_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEB
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_gt_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEC
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_lg_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xED
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_ge_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEE
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_tru_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEF
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 2)
		writeVRegU64(state, 1, 0, 2)
		writeVRegU64(state, 1, 2, 1)
		writeVRegU64(state, 2, 0, 2)
		writeVRegU64(state, 2, 2, 2)

		alu.Run(state)

		Expect(state.vcc).To(Equal(uint64(0x7)))
	})

})
