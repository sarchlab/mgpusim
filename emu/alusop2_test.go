package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v2/insts"
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

	It("should run S_ADD_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 0

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1))   // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31+15)) // SRC1
		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).
			To(Equal(uint32(14)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run S_SUB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 1
		sp := state.scratchpad.AsSOP2()

		sp.SRC0 = 10
		sp.SRC1 = 5
		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(5)))
	})

	It("should run S_SUB_U32 with carry out", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 1
		sp := state.scratchpad.AsSOP2()

		sp.SRC0 = 5
		sp.SRC1 = 10
		alu.Run(state)

		Expect(sp.DST).To(Equal(^uint64(0) - 4))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_ADD_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = uint64((int32ToBits(-6)))
		sp.SRC1 = 15

		alu.Run(state)

		Expect(asInt32(uint32(sp.DST))).To(Equal(int32(-21)))
		Expect(sp.SCC).To(Equal(byte(0)))
	})

	It("should run S_SUB_I32, when overflow and src1 is positive", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x7ffffffe
		sp.SRC1 = 0xfffffffc

		alu.Run(state)

		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_SUB_I32, when overflow and src1 is negtive", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x80000001
		sp.SRC1 = 10

		alu.Run(state)

		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 4

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1)) // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31))  // SRC1
		state.scratchpad[24] = 1                                  // SCC

		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).To(Equal(uint32(0)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run S_SUBB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		sp := state.Scratchpad().AsSOP2()

		sp.SRC0 = 10
		sp.SRC1 = 5
		sp.SCC = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(4)))
		Expect(sp.SCC).To(Equal(uint8(0)))
	})

	It("should run S_SUBB_U32 with carry out", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		sp := state.Scratchpad().AsSOP2()

		sp.SRC0 = 5
		sp.SRC1 = 10
		sp.SCC = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(^uint64(0) - 5))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_SUBB_U32 with carry out", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		sp := state.Scratchpad().AsSOP2()

		sp.SRC0 = 0
		sp.SRC1 = 0
		sp.SCC = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(^uint64(0)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_MIN_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 6
		sp := state.Scratchpad().AsSOP2()

		sp.SRC0 = uint64(int32ToBits(-1))
		sp.SRC1 = uint64(int32ToBits(5))

		alu.Run(state)

		Expect(asInt32(uint32(sp.DST))).To(Equal(int32(-1)))
	})

	It("should run S_MIN_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 7

		sp := state.scratchpad.AsSOP2()
		sp.SRC0 = 1
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(1)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_MIN_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 7

		sp := state.scratchpad.AsSOP2()
		sp.SRC0 = 2
		sp.SRC1 = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(1)))
		Expect(sp.SCC).To(Equal(uint8(0)))
	})

	It("should run S_MAX_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 8
		sp := state.Scratchpad().AsSOP2()

		sp.SRC0 = uint64(int32ToBits(-1))
		sp.SRC1 = uint64(int32ToBits(5))

		alu.Run(state)

		Expect(asInt32(uint32(sp.DST))).To(Equal(int32(5)))
	})

	It("should run S_MAX_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 9

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xff
		sp.SRC1 = 0xffff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xffff)))
		Expect(sp.SCC).To(Equal(uint8(0)))
	})

	It("should run S_MAX_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 9

		sp := state.Scratchpad().AsSOP2()
		sp.SRC1 = 0xff
		sp.SRC0 = 0xffff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xffff)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_CSELECT_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 10

		sp := state.Scratchpad().AsSOP2()
		sp.SRC1 = 0xff
		sp.SRC0 = 0xffff
		sp.SCC = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xffff)))
	})

	It("should run S_AND_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 15

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xf0
		sp.SRC1 = 0xff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xff)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_XOR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 16

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xf0
		sp.SRC1 = 0xff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0x0f)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_XOR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 17

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xf0
		sp.SRC1 = 0xff

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0x0f)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_ANDN2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 19

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0xab
		sp.SRC1 = 0x0f

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0xa0)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_LSHL_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 28

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 128
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(512)))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_LSHL_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 29

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x8000000000000000
		sp.SRC1 = 1

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0)))
		Expect(sp.SCC).To(Equal(uint8(0)))
	})

	It("should run S_LSHR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 30

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x20
		sp.SRC1 = 0x64

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0x02)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_LSHR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 31

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x20
		sp.SRC1 = 0x44

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(0x02)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should run S_ASHR_I32 (Negative)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
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
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 32

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = int64ToBits(128)
		sp.SRC1 = 2

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(int32ToBits(32))))
		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run S_BFM_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 34

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0x24
		sp.SRC1 = 0x64

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(int32ToBits(240))))
	})

	It("should run S_MUL_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 36

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 5
		sp.SRC1 = 7

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(35)))
	})

	It("should run S_MUL_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 38

		sp := state.Scratchpad().AsSOP2()
		sp.SRC0 = 0b1111_0100
		sp.SRC1 = 0b000000000_0000001_00000000000_00010

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(1)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

})
