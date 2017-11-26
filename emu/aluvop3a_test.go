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

	It("should run V_MUL_LO_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 645

		sp := state.Scratchpad().AsVOP3A()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(i)
			sp.SRC1[i] = uint64(2)
		}
		sp.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(sp.DST[i]).To(Equal(uint64(i * 2)))
		}
	})

	It("should run V_MUL_HI_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 646

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC0[0] = uint64(0x80000000)
		sp.SRC1[0] = uint64(2)
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(1)))

	})

	It("should run V_LSHLREV_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 655

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC1[0] = uint64(0x0000000000010000)
		sp.SRC0[0] = uint64(3)
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x0000000000080000)))
	})

	It("should run V_ASHRREV_I64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 657

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC1[0] = uint64(0x0000000000010000)
		sp.SRC1[1] = uint64(0xffffffff00010000)
		sp.SRC0[0] = 4
		sp.SRC0[1] = 4
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x0000000000001000)))
		Expect(sp.DST[1]).To(Equal(uint64(0xfffffffff0001000)))
	})
})
