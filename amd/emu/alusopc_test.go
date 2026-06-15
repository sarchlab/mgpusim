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

	It("should run S_CMP_EQ_I32 when input is not equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 0
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_EQ_I32 when input is equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 0
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LG_I32 when condition holds", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LG_I32 when condition does not hold", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_GT_I32 when condition holds", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 2
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(2))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_GT_I32 when condition does not hold", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 2
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_GE_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LT_I32 when condition holds", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 4
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(uint64(int32ToBits(-2))))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(uint64(int32ToBits(-1))))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LT_I32 when condition does not hold", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 4
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(int64ToBits(-1)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(int64ToBits(-1)))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_LE_I32 when condition holds", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(uint64(int32ToBits(-2))))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(uint64(int32ToBits(-1))))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LE_I32 when condition does not hold", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(int64ToBits(-1)))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(int64ToBits(-2)))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_EQ_U32 when input is not equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 6
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_EQ_U32 when input is equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 6
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LG_U32 when condition holds", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LG_U32 when condition does not hold", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(0)))
	})

	It("should run S_CMP_GT_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(2))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})

	It("should run S_CMP_LT_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.SOPC
		state.inst.Opcode = 10
		state.inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewSRegOperand(0, 2, 1)

		state.WriteReg(insts.SReg(0), 1, 0, insts.Uint64ToBytes(1))
		state.WriteReg(insts.SReg(2), 1, 0, insts.Uint64ToBytes(2))

		alu.Run(state)

		Expect(state.SCC()).To(Equal(byte(1)))
	})
})
