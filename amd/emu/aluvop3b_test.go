package emu

import (
	"math"

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

	It("should run V_ADD_U32 VOP3b", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 281
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 3

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 1, 0, 0xffffffff)
		writeVRegU32(state, 1, 1, 2)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(3)))
		Expect(state.ReadOperand(state.inst.Dst, 1) & 0xffffffff).To(Equal(uint64(1)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(0x2)))
	})

	It("should run V_SUB_U32 VOP3b", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 282
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 3

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 1, 0, 0xffffffff)
		writeVRegU32(state, 1, 1, 2)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0) & 0xffffffff).To(Equal(uint64(0xffffffff)))
		Expect(state.ReadOperand(state.inst.Dst, 1) & 0xffffffff).To(Equal(uint64(0xfffffffd)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(0x1)))
	})

	It("should run V_SUBREV_U32 VOP3b", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 283
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 3

		writeVRegU32(state, 0, 0, 2)
		writeVRegU32(state, 0, 1, 0xffffffff)
		writeVRegU32(state, 1, 0, 2)
		writeVRegU32(state, 1, 1, 0x0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0xfffffffd)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(0xfffffffe)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(2)))
	})

	It("should run V_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 284
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewSRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x3

		writeVRegU32(state, 0, 0, 0xfffffffd)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 1, 0, 0xfffffffd)
		writeVRegU32(state, 1, 1, 1)
		// src2 = s[4:5] = bitmask with bit 0 set
		copy(state.sRegFile[4*4:], insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(0xfffffffe)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_SUBB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 285
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewSRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x3

		writeVRegU32(state, 0, 0, 0x1)
		writeVRegU32(state, 0, 1, 0x2)
		writeVRegU32(state, 1, 0, 0xfffffffd)
		writeVRegU32(state, 1, 1, 0x1)
		// src2 = s[4:5] = bitmask with bit 0 set
		copy(state.sRegFile[4*4:], insts.Uint64ToBytes(0x1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0xfffffffe)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(0xfffffffc)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_SUBBREV_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 286
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewSRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x3

		writeVRegU32(state, 0, 0, 0x2)
		writeVRegU32(state, 0, 1, 0x1)
		writeVRegU32(state, 1, 0, 0x1)
		writeVRegU32(state, 1, 1, 0xfffffffd)
		// src2 = s[4:5] = bitmask with bit 0 set
		copy(state.sRegFile[4*4:], insts.Uint64ToBytes(0x1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0xfffffffe)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(0xfffffffc)))
		Expect(state.ReadOperand(state.inst.SDst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_DIV_SCALE_F64", func() {
		// Need more test case
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3b
		state.inst.Opcode = 481
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Src2 = insts.NewVRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 6, 2)
		state.inst.SDst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, 0x3FF0000000000000)
		writeVRegU64(state, 0, 2, 0x0008A00000000000)
		writeVRegU64(state, 0, 4, 0x0008A00000000000)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(math.Pow(2.0, 128))))
	})

})
