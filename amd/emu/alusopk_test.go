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

		state = newMockInstState()
	})

	It("should run s_movk_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 0
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(-12)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 0, 1), 0)
		Expect(asInt16(uint16(dst))).To(Equal(int16(-12)))
	})

	It("should run s_cmovk_i32 with SCC = 1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 1
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(-12)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 0, 1), 0)
		Expect(asInt16(uint16(dst))).To(Equal(int16(-12)))
	})

	It("should run s_cmpk_eq_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 2
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(200)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(200))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(uint8(1)))
	})

	It("should run s_cmpk_lg_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 3
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(100)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(200))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(uint8(1)))
	})

	It("should run s_mulk_i32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 15
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(100)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(200))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 0, 1), 0)
		Expect(dst).To(Equal(uint64(20000)))
	})

	It("should run s_mulk_i32 with negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPK
		state.inst.Opcode = 15
		state.inst.SImm16 = insts.NewIntOperand(0, int64(int16ToBits(-100)))
		state.inst.Dst = insts.NewSRegOperand(0, 0, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(200))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 0, 1), 0)
		// 32-bit result stored, check as int32
		Expect(asInt32(uint32(dst))).To(Equal(int32(-20000)))
	})

})
