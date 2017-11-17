package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

var _ = Describe("ALU", func() {

	var (
		alu     *ALU
		state   *mockInstState
		storage *mem.Storage
	)

	BeforeEach(func() {
		storage = mem.NewStorage(1 * mem.GB)
		alu = new(ALU)
		alu.Storage = storage

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run S_ADD_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 0

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1))   // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31+15)) // SRC1
		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).To(Equal(uint32(14)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run S_SUB_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 10
		sp.SRC1 = 6

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(4)))
	})

	It("should run S_SUB_I32, when input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = uint64((int32ToBits(-6)))
		sp.SRC1 = 15

		alu.Run(state)

		Expect(asInt32(uint32(sp.DST))).To(Equal(int32(-21)))
	})

	It("should run S_SUB_I32, when overflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 3

		// sp := state.Scratchpad().AsSOP2()
		// sp.

	})

	It("should run S_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 4

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1)) // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31))  // SRC1
		state.scratchpad[24] = 1                                  // SCC

		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).To(Equal(uint32(0)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run S_AND_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 13

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xff
		sp.SRC1 = 0xffff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xff)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

})
