package emu

import (
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
		alu.LDS = make([]byte, 4096)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
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

		Expect(insts.BytesToUint32(alu.LDS[108:])).To(Equal(uint32(1)))
		Expect(insts.BytesToUint32(alu.LDS[112:])).To(Equal(uint32(2)))
		Expect(insts.BytesToUint32(alu.LDS[124:])).To(Equal(uint32(3)))
		Expect(insts.BytesToUint32(alu.LDS[128:])).To(Equal(uint32(4)))
	})

})
