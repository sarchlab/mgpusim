package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALUImpl(nil)

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

})
