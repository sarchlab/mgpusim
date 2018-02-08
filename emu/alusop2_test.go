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

	It("should run S_ADD_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 2

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xffffffff
		sp.SRC1 = 3

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(2)))
		Expect(sp.SCC).To(Equal(byte(1)))

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
		Expect(sp.SCC).To(Equal(byte(0)))
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
		Expect(sp.SCC).To(Equal(byte(0)))
	})

	It("should run S_SUB_I32, when overflow and src1 is positve", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x7ffffffe
		sp.SRC1 = 0xfffffffc

		alu.Run(state)

		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_SUB_I32, when overflow and src1 is negtive", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x80000001
		sp.SRC1 = 10

		alu.Run(state)

		Expect(sp.SCC).To(Equal(byte(1)))
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

	It("should run S_AND_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 12

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xff
		sp.SRC1 = 0xffff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xff)))
		Expect(sp.SCC).To(Equal(uint8(1)))
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

	It("should run S_OR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 15

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xf0
		sp.SRC1 = 0xff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xff)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})


	It("should run S_XOR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 17

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xf0
		sp.SRC1 = 0xff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0x0f)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})


	It("should run S_LSHR_B32", func() {
	   	state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 30

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 400
		sp.SRC1 = 100

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(6400)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_LSHL_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 29

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 128
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(512)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_LSHL_B64 (To zero)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 29

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x8000000000000000
		sp.SRC1 = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0)))
		Expect(sp.SCC).To(Equal(uint8(0)))
	})

	It("should run S_ASHR_I32 (Negative)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 32

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = int64ToBits(-128)
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(int32ToBits(-32))))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_ASHR_I32 (Positive)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 32

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = int64ToBits(128)
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(int32ToBits(32))))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_MUL_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 36

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 5
		sp.SRC1 = 7

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(35)))
	})

})
