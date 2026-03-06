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

	It("should run V_CNDMASK_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 0
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 3
		state.vcc = 1

		// lane 0: src0=1, src1=3
		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(1))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(3))
		// lane 1: src0=2, src1=4
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(2))
		copy(state.vRegFile[1*256*4+1*4:], insts.Uint32ToBytes(4))

		alu.Run(state)

		// lane 0: VCC bit 0 = 1, so pick src1 = 3
		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(3)))
		// lane 1: VCC bit 1 = 0, so pick src0 = 2
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(2)))
	})

	It("should run V_ADD_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(
			Equal(math.Float32bits(float32(5.1))))
	})

	It("should run V_SUB_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 2
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(math.Float32frombits(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(
			BeNumerically("~", -1.1, 1e-4))
	})

	It("should run V_SUBREV_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 3
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(math.Float32frombits(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(
			BeNumerically("~", 1.1, 1e-4))
	})

	It("should run V_MUL_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(
			Equal(math.Float32bits(float32(6.2))))
	})

	It("should run V_MUL_I32_I24", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 6
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(int32ToBits(-10)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(int32ToBits(20)))

		alu.Run(state)

		Expect(int32(state.ReadOperand(state.inst.Dst, 0) & 0xffffffff)).To(Equal(int32(-200)))
	})

	It("should run V_MUL_U32_U24", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(2))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x1000001))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(2)))
	})

	It("should run V_MIN_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 10
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(
			Equal(math.Float32bits(float32(2.0))))
	})

	It("should run V_MAX_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 11
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(math.Float32bits(3.1)))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(
			Equal(math.Float32bits(float32(3.1))))
	})

	It("should run V_MIN_U32, with src0 > src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 14
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x20))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x20)))
	})

	It("should run V_MIN_U32, with src0 = src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 14
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x64))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x64)))
	})

	It("should run V_MIN_U32, with src0 < src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 14
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x20))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x23))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x20)))
	})

	It("should run V_MAX_U32, with src0 > src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 15
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x20))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x64)))
	})

	It("should run V_MAX_U32, with src0 = src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 15
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x64))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x64)))
	})

	It("should run V_MAX_U32, with src0 < src1", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 15
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x20))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x23))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x23)))
	})

	It("should run V_LSHRREV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 16
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x20))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x02)))
	})

	It("should run V_ASHRREV_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 17
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(97))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(int32ToBits(-64)))

		alu.Run(state)

		Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(Equal(int32(-32)))
	})

	It("should run V_LSHLREV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 18
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x02))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x20)))
	})

	It("should run V_LSHLREV_B16", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 42
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0x64))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x04))

		alu.Run(state)

		Expect(uint16(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint16(0x40)))
	})

	It("should run V_AND_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 19
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(2)) // 10
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(3)) // 11

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(2)))
	})

	It("should run V_AND_B32 SDWA", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 19
		state.inst.IsSdwa = true
		state.inst.Src0Sel = insts.SDWASelectByte0
		state.inst.Src1Sel = insts.SDWASelectByte3
		state.inst.DstSel = insts.SDWASelectWord1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0xfedcba98))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x12345678))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x00100000)))
	})

	It("should run V_OR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 20
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(2)) // 10
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(3)) // 11

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(3)))
	})

	It("should run V_OR_B32 SDWA", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 20
		state.inst.IsSdwa = true
		state.inst.Src0Sel = insts.SDWASelectByte0
		state.inst.Src1Sel = insts.SDWASelectByte3
		state.inst.DstSel = insts.SDWASelectWord1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0xfedcba98))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x12345678))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x009a0000)))
	})

	It("should run V_XOR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 21
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(2)) // 10
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(3)) // 11

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(1)))
	})

	It("should run V_XOR_B32 SDWA", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 21
		state.inst.IsSdwa = true
		state.inst.Src0Sel = insts.SDWASelectByte0
		state.inst.Src1Sel = insts.SDWASelectByte3
		state.inst.DstSel = insts.SDWASelectWord1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(0xfedcba98))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(0x12345678))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0x008a0000)))
	})

	It("should run V_MAC_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 22
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(float32ToBits(4)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(float32ToBits(16)))
		copy(state.vRegFile[0*256*4+2*4:], insts.Uint32ToBytes(float32ToBits(1024)))

		alu.Run(state)

		Expect(asFloat32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(
			Equal(float32(1024.0 + 16.0*4.0)))
	})

	It("should run V_MADAK_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 24
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.inst.Src2 = &insts.Operand{
			OperandType:     insts.LiteralConstant,
			LiteralConstant: float32ToBits(1024),
		}
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(float32ToBits(4)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(float32ToBits(16)))

		alu.Run(state)

		Expect(asFloat32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(
			Equal(float32(1024.0 + 16.0*4.0)))
	})

	It("should run V_ADD_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 25
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0xffffffffffffffff

		for i := 0; i < 64; i++ {
			copy(state.vRegFile[i*256*4+0*4:], insts.Uint32ToBytes(int32ToBits(-100)))
			copy(state.vRegFile[i*256*4+1*4:], insts.Uint32ToBytes(int32ToBits(10)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, i)))).To(Equal(int32(-90)))
		}
	})

	It("should run V_ADD_I32_SDWA", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 25
		state.inst.IsSdwa = true
		state.inst.Src0Sel = insts.SDWASelectByte0
		state.inst.Src1Sel = insts.SDWASelectByte0
		state.inst.DstSel = insts.SDWASelectDWord
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0xffffffffffffffff

		for i := 0; i < 64; i++ {
			copy(state.vRegFile[i*256*4+0*4:], insts.Uint32ToBytes(int32ToBits(-100)))
			copy(state.vRegFile[i*256*4+1*4:], insts.Uint32ToBytes(int32ToBits(10)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, i)))).To(Equal(int32(166)))
		}
	})

	It("should run V_SUB_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 26
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(10))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(4))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(6)))
		Expect(state.VCC()).To(Equal(uint64(0)))
	})

	It("should run V_SUB_I32, when underflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 26
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(4))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(10))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0xfffffffa)))
		Expect(state.VCC()).To(Equal(uint64(1)))
	})

	It("should run V_SUBREV_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 27
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(4))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(10))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(6)))
		Expect(state.VCC()).To(Equal(uint64(0)))
	})

	It("should run V_SUBREV_I32, when underflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 27
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(10))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(4))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0xfffffffa)))
		Expect(state.VCC()).To(Equal(uint64(1)))
	})

	It("should run V_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 28
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1
		state.vcc = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(uint32(math.MaxUint32-10)))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(10))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0)))
		Expect(state.VCC()).To(Equal(uint64(1)))
	})

	It("should run V_SUBB_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 29
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x3
		state.vcc = 3

		// lane 0: src0=10, src1=5
		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(10))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(5))
		// lane 1: src0=5, src1=10
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(5))
		copy(state.vRegFile[1*256*4+1*4:], insts.Uint32ToBytes(10))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(4)))
		Expect(uint32(state.ReadOperand(state.inst.Dst, 1))).To(Equal(^uint32(0) - 5))
		Expect(state.VCC()).To(Equal(uint64(2)))
	})

	It("should run V_SUBBREV_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 30
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1
		state.vcc = 0

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(10))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(11))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(1)))
		Expect(state.VCC()).To(Equal(uint64(0)))
	})

	It("should run V_SUBBREV_U32, when underflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP2
		state.inst.Opcode = 30
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 1
		state.vcc = 1

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(10))
		copy(state.vRegFile[0*256*4+1*4:], insts.Uint32ToBytes(4))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(uint32(0xfffffff9)))
		Expect(state.VCC()).To(Equal(uint64(1)))
	})

})
