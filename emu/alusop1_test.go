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

	It("should run s_and_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop1
		state.inst.Opcode = 32

		sp := state.Scratchpad().AsSOP1()
		sp.EXEC = 0xffffffff00000000
		sp.SRC0 = 0x0000ffffffff0000

		alu.Run(state)

		Expect(sp.EXEC).To(Equal(uint64(0x0000ffff00000000)))
		Expect(sp.DST).To(Equal(uint64(0x0000ffff00000000)))
		Expect(sp.SCC).To(Equal(byte(0x1)))
	})

})
