package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("ALU", func() {

	var (
		alu   *ALU
		state *mockInstState
	)

	BeforeEach(func() {
		alu = new(ALU)

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run v_cmp_ne_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vopc
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
})
