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

	It("should run S_ADD_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 0
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(1<<31-1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(1<<31+15))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(uint32(14)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_SUB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(10))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(5))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(5)))
	})

	It("should run S_SUB_U32 with carry out", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(5))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(10))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		// 32-bit register stores uint32(5-10) = 0xfffffffb, read back as uint64 = 0xfffffffb
		Expect(uint32(dst)).To(Equal(uint32(0xfffffffb)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_ADD_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 2
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(0xffffffff))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(3))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(uint32(2)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_SUB_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(10))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(6))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(uint32(4)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_SUB_I32, when input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(int32ToBits(-6)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(15))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(asInt32(uint32(dst))).To(Equal(int32(-21)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_SUB_I32, when overflow and src1 is positive", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0x7ffffffe))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xfffffffc))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_SUB_I32, when overflow and src1 is negtive", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0x80000001))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(10))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 4
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(1<<31-1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(1<<31))
		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(uint32(0)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_SUBB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(10))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(5))
		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(uint32(4)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_SUBB_U32 with carry out", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(5))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(10))
		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		// 5 - 10 - 1 = -6, stored in 32-bit = 0xfffffffa
		Expect(uint32(dst)).To(Equal(uint32(0xfffffffa)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_SUBB_U32 with carry out (zero case)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0))
		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		// 0 - 0 - 1 = -1, stored in 32-bit = 0xffffffff
		Expect(uint32(dst)).To(Equal(uint32(0xffffffff)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_MIN_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 6
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(int32ToBits(-1)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(int32ToBits(5)))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(asInt32(uint32(dst))).To(Equal(int32(-1)))
	})

	It("should run S_MIN_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(1)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_MIN_U32 (second smaller)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(2))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(1)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_MAX_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(int32ToBits(-1)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(int32ToBits(5)))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(asInt32(uint32(dst))).To(Equal(int32(5)))
	})

	It("should run S_MAX_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 9
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xff))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xffff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xffff)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_MAX_U32 (first larger)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 9
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xffff))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xffff)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CSELECT_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 10
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xffff))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xff))
		state.SetSCC(1)

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xffff)))
	})

	It("should run S_AND_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 12
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xff))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xffff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xff)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_AND_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 13
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0xff))
		state.WriteReg(insts.SReg(2), 2, 0, insts.Uint64ToBytes(0xffff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xff)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_OR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 15
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0xf0))
		state.WriteReg(insts.SReg(2), 2, 0, insts.Uint64ToBytes(0xff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xff)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_XOR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 16
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0xf0))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0xff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0x0f)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_XOR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 17
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0xf0))
		state.WriteReg(insts.SReg(2), 2, 0, insts.Uint64ToBytes(0xff))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0x0f)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_ANDN2_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 19
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0xab))
		state.WriteReg(insts.SReg(2), 2, 0, insts.Uint64ToBytes(0x0f))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0xa0)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_LSHL_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 28
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(128))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(512)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_LSHL_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 29
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(128))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(512)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_LSHL_B64 (To zero)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 29
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x8000000000000000))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0)))
		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_LSHR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 30
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0x20))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0x64))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0x02)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_LSHR_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 31
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 2)

		state.WriteReg(insts.SReg(0), 2, 0, insts.Uint64ToBytes(0x20))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0x44))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(0x02)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_ASHR_I32 (Negative)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 32
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(int32ToBits(-128)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(int32ToBits(-32)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_ASHR_I32 (Positive)", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 32
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(int32ToBits(128)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(int32ToBits(32)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_BFM_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 34
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0x24))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0x64))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(dst)).To(Equal(int32ToBits(240)))
	})

	It("should run S_MUL_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 36
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(5))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(7))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(35)))
	})

	It("should run S_BFE_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOP2
		state.inst.Opcode = 38
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 4, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(0b1111_0100))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(0b000000000_0000001_00000000000_00010))

		alu.Run(state)

		dst := state.ReadOperand(state.inst.Dst, 0)
		Expect(dst).To(Equal(uint64(1)))
		Expect(state.SCC()).To(Equal(byte(1)))
	})

})
