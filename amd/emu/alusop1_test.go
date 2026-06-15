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

	It("should run s_mov_b32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 0
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(0xffff0000))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 2, 1), 0)
		Expect(dst).To(Equal(uint64(0xffff0000)))
	})

	It("should run s_mov_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0x0000ffffffff0000)))
	})

	It("should run s_not_u32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 4
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(0xff))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 2, 1), 0)
		// ReadOperand returns 32-bit register zero-extended to 64-bit
		// ~0xff = 0xffffffffffffff00, stored in 32-bit dst = 0xffffff00
		Expect(dst).To(Equal(uint64(0xffffff00)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_brev_b32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xffff))

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 2, 1), 0)
		Expect(dst).To(Equal(uint64(0x00000000ffff0000)))
	})

	It("should run s_get_pc_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 28
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)

		state.SetPC(0xffffffff00000000)

		alu.Run(state)

		dst := state.ReadOperand(insts.NewSRegOperand(0, 0, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000004)))
	})

	It("should run s_and_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 32
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0x0000ffff00000000)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_or_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 33
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0xffffffffffff0000)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_xor_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 34
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0xffff0000ffff0000)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_andn2_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 35
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0x00000000ffff0000)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_orn2_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 36
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0x0000ffffffffffff)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_nand_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 37
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0xffff0000ffffffff)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_nor_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 38
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0x000000000000ffff)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

	It("should run s_nxor_saveexec_b64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP1
		state.inst.Opcode = 39
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.SetEXEC(0xffffffff00000000)
		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x0000ffffffff0000))

		alu.Run(state)

		Expect(state.EXEC()).To(Equal(uint64(0x0000ffff0000ffff)))
		dst := state.ReadOperand(insts.NewSRegOperand(0, 4, 2), 0)
		Expect(dst).To(Equal(uint64(0xffffffff00000000)))
		Expect(state.SCC()).To(Equal(byte(0x1)))
	})

})
