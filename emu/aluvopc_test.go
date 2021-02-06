package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALU(nil)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run v_cmp_lt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x41

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_eq_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x42

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(-2.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_le_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x43

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(-2.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_gt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x44

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_lg_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x45

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_ge_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x46

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_nge_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x49

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_nlg_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4A

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_ngt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4B

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_nle_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4C

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_neq_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4D

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_nlt_f32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0x4E

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = uint64(math.Float32bits(-1.2))
		sp.SRC1[0] = uint64(math.Float32bits(-1.2))
		sp.SRC0[1] = uint64(math.Float32bits(-2.5))
		sp.SRC1[1] = uint64(math.Float32bits(0.0))
		sp.SRC0[2] = uint64(math.Float32bits(1.5))
		sp.SRC1[2] = uint64(math.Float32bits(0.0))

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_lt_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC1

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC0[2] = 1
		sp.SRC0[3] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC1[2] = 0
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x8)))
	})

	It("should run v_cmp_le_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC3

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.SRC0[3] = 1
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x9)))
	})

	It("should run v_cmp_gt_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC4

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC0[2] = 1
		sp.SRC0[3] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC1[2] = 0
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_lg_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC5

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC0[2] = 1
		sp.SRC0[3] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC1[2] = 0
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0xE)))
	})

	It("should run v_cmp_ge_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC6

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0
		sp.SRC0[3] = 1
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x7)))
	})

	It("should run v_cmp_lt_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xC9

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xF
		sp.SRC0[0] = 1
		sp.SRC0[1] = uint64(int32ToBits(-1))
		sp.SRC0[2] = 1
		sp.SRC0[3] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = uint64(int32ToBits(-2))
		sp.SRC1[2] = 0
		sp.SRC1[3] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x8)))
	})

	It("should run v_cmp_eq_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCA

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC0[1] = 1
		sp.SRC0[2] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = 2
		sp.SRC1[2] = 0

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_le_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCB

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xffffffffffffffff
		sp.SRC0[0] = 1
		sp.SRC0[1] = 1
		sp.SRC0[2] = 1
		sp.SRC1[0] = 1
		sp.SRC1[1] = 2
		sp.SRC1[2] = 0

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0xfffffffffffffffb)))
	})

	It("should run v_cmp_gt_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCC

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_ne_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCD

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0xffffffffffffffff
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 0
		sp.SRC1[1] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x0000000000000002)))
	})

	It("should run v_cmp_ge_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xCE

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 1
		sp.SRC0[1] = 1
		sp.SRC1[1] = 2
		sp.SRC0[2] = 1
		sp.SRC1[2] = 0

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_f_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xE8

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x0)))
	})

	It("should run v_cmp_lt_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xE9

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x3
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_eq_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEA

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_le_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEB

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_gt_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEC

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_lg_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xED

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x3)))
	})

	It("should run v_cmp_ge_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEE

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_tru_u64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOPC
		state.inst.Opcode = 0xEF

		sp := state.Scratchpad().AsVOPC()
		sp.EXEC = 0x7
		sp.SRC0[0] = 1
		sp.SRC1[0] = 2
		sp.SRC0[1] = 2
		sp.SRC1[1] = 1
		sp.SRC0[2] = 2
		sp.SRC1[2] = 2

		alu.Run(state)

		Expect(sp.VCC).To(Equal(uint64(0x7)))
	})

})
