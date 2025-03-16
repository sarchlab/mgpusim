package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
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

	It("should run s_movk_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 0

		sp := state.Scratchpad().AsSOPK()
		sp.IMM = uint64(int16ToBits(-12))

		alu.Run(state)

		Expect(asInt16(uint16(sp.DST))).To(Equal(int16(-12)))
	})

	It("should run s_cmpk_lg_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsSOPK()
		sp.IMM = uint64(int16ToBits(100))
		sp.DST = 200

		alu.Run(state)

		Expect(sp.SCC).To(Equal(uint8(1)))
	})

	It("should run s_mulk_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 15

		sp := state.Scratchpad().AsSOPK()
		sp.IMM = uint64(int16ToBits(100))
		sp.DST = 200

		alu.Run(state)

		Expect(sp.DST).To(Equal(uint64(20000)))
	})

	It("should run s_mulk_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 15

		sp := state.Scratchpad().AsSOPK()
		sp.IMM = uint64(int16ToBits(-100))
		sp.DST = 200

		alu.Run(state)

		Expect(asInt64(sp.DST)).To(Equal(int64(-20000)))
	})

})
