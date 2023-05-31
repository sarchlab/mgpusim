package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALUImpl
		state *mockInstState
	)

	BeforeEach(func() {
		alu = NewALU(nil)
		alu.lds = make([]byte, 4096)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run DS_WRITE_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 13
		state.inst.Offset0 = 0

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x01
		sp.ADDR[0] = 100
		sp.DATA[0] = 1

		alu.Run(state)

		lds := alu.LDS()
		Expect(insts.BytesToUint32(lds[100:])).To(Equal(uint32(1)))
	})

	It("should run DS_WRITE2_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 14
		state.inst.Offset0 = 0
		state.inst.Offset1 = 4

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x01
		sp.ADDR[0] = 100
		sp.DATA[0] = 1
		sp.DATA1[0] = 2

		alu.Run(state)

		lds := alu.LDS()
		Expect(insts.BytesToUint32(lds[100:])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(lds[116:])).To(Equal(uint32(2)))
	})

	It("should run DS_READ_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 54

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x1
		sp.ADDR[0] = 100

		lds := alu.LDS()
		copy(lds[100:], insts.Uint32ToBytes(12))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint32(12)))
	})

	It("should run DS_READ2_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 55
		state.inst.Offset0 = 0
		state.inst.Offset1 = 4

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x1
		sp.ADDR[0] = 100

		lds := alu.LDS()
		copy(lds[100:], insts.Uint32ToBytes(1))
		copy(lds[116:], insts.Uint32ToBytes(2))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint32(1)))
		Expect(sp.DST[1]).To(Equal(uint32(2)))
	})

	It("should run DS_WRITE2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 78
		state.inst.Offset0 = 1
		state.inst.Offset1 = 3

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x1
		sp.ADDR[0] = 100
		sp.DATA[0] = 1
		sp.DATA[1] = 2
		sp.DATA1[0] = 3
		sp.DATA1[1] = 4

		alu.Run(state)

		lds := alu.LDS()
		Expect(insts.BytesToUint32(lds[108:])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(lds[112:])).To(Equal(uint32(2)))
		Expect(insts.BytesToUint32(lds[124:])).To(Equal(uint32(3)))
		Expect(insts.BytesToUint32(lds[128:])).To(Equal(uint32(4)))
	})

	It("should run DS_READ_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 118

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x1
		sp.ADDR[0] = 100

		lds := alu.LDS()
		copy(lds[100:], insts.Uint64ToBytes(12))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint32(12)))
	})

	It("should run DS_READ2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.DS
		state.inst.Opcode = 119
		state.inst.Offset0 = 1
		state.inst.Offset1 = 3

		sp := state.scratchpad.AsDS()
		sp.EXEC = 0x1
		sp.ADDR[0] = 100

		lds := alu.LDS()
		copy(lds[108:], insts.Uint32ToBytes(12))
		copy(lds[124:], insts.Uint32ToBytes(156))

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint32(12)))
		Expect(sp.DST[2]).To(Equal(uint32(156)))
	})

})
