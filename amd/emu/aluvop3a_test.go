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

	It("should run v_cmp_lt_f32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 65
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 0, 1, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 1, 0, math.Float32bits(float32(-2.5)))
		writeVRegU32(state, 1, 1, math.Float32bits(float32(0.0)))
		writeVRegU32(state, 2, 0, math.Float32bits(float32(1.5)))
		writeVRegU32(state, 2, 1, math.Float32bits(float32(0.0)))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x2)))
	})

	It("should run v_cmp_gt_f32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 68
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 0, 1, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 1, 0, math.Float32bits(float32(-2.5)))
		writeVRegU32(state, 1, 1, math.Float32bits(float32(0.0)))
		writeVRegU32(state, 2, 0, math.Float32bits(float32(1.5)))
		writeVRegU32(state, 2, 1, math.Float32bits(float32(0.0)))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x4)))
	})

	It("should run v_cmp_nlt_f32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 78
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 0, 1, math.Float32bits(float32(-1.2)))
		writeVRegU32(state, 1, 0, math.Float32bits(float32(-2.5)))
		writeVRegU32(state, 1, 1, math.Float32bits(float32(0.0)))
		writeVRegU32(state, 2, 0, math.Float32bits(float32(1.5)))
		writeVRegU32(state, 2, 1, math.Float32bits(float32(0.0)))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x5)))
	})

	It("should run v_cmp_lt_i32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 193
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0xf

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(1))
		writeVRegU32(state, 1, 0, int32ToBits(2))
		writeVRegU32(state, 1, 1, int32ToBits(1))
		writeVRegU32(state, 2, 0, int32ToBits(0))
		writeVRegU32(state, 2, 1, int32ToBits(-1))
		writeVRegU32(state, 3, 0, int32ToBits(-1))
		writeVRegU32(state, 3, 1, int32ToBits(-1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x1)))
	})

	It("should run v_cmp_le_i32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 195
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0xf

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(1))
		writeVRegU32(state, 1, 0, int32ToBits(2))
		writeVRegU32(state, 1, 1, int32ToBits(1))
		writeVRegU32(state, 2, 0, int32ToBits(0))
		writeVRegU32(state, 2, 1, int32ToBits(-1))
		writeVRegU32(state, 3, 0, int32ToBits(-1))
		writeVRegU32(state, 3, 1, int32ToBits(-1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x9)))
	})

	It("should run v_cmp_gt_i32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 196
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0xf

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(1))
		writeVRegU32(state, 1, 0, int32ToBits(2))
		writeVRegU32(state, 1, 1, int32ToBits(1))
		writeVRegU32(state, 2, 0, int32ToBits(0))
		writeVRegU32(state, 2, 1, int32ToBits(-1))
		writeVRegU32(state, 3, 0, int32ToBits(-1))
		writeVRegU32(state, 3, 1, int32ToBits(-1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x6)))
	})

	It("should run v_cmp_ge_i32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 198
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0xf

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(1))
		writeVRegU32(state, 1, 0, int32ToBits(2))
		writeVRegU32(state, 1, 1, int32ToBits(1))
		writeVRegU32(state, 2, 0, int32ToBits(0))
		writeVRegU32(state, 2, 1, int32ToBits(-1))
		writeVRegU32(state, 3, 0, int32ToBits(-1))
		writeVRegU32(state, 3, 1, int32ToBits(-1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0xe)))
	})

	It("should run V_CMP_LT_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 201
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(2)))
	})

	It("should run V_CMP_EQ_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 202
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 3

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_CMP_LE_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 203
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(3)))
	})

	It("should run V_CMP_GT_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 204
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(4)))
	})

	It("should run V_CMP_LG_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 205
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(6)))
	})

	It("should run V_CMP_GE_U32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 206
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 1)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		writeVRegU32(state, 2, 0, 1)
		writeVRegU32(state, 2, 1, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(5)))
	})

	It("should run V_CMP_LT_U64 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 233
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewSRegOperand(0, 0, 2)
		state.exec = 0x7

		writeVRegU64(state, 0, 0, 1)
		writeVRegU64(state, 0, 2, 1)
		writeVRegU64(state, 1, 0, 1)
		writeVRegU64(state, 1, 2, 2)
		writeVRegU64(state, 2, 0, 1)
		writeVRegU64(state, 2, 2, 0)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(2)))
	})

	It("should run V_CNDMASK_B32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 256
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewSRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.exec = 3

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 1, 0, 1)
		writeVRegU32(state, 1, 1, 2)
		// src2 = s[0:1] = bitmask, set bit 0 = 1
		copy(state.sRegFile[0:], insts.Uint64ToBytes(1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(2)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(1)))
	})

	It("should run V_SUB_F32 VOP3a", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 258
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.exec = 0x1

		writeVRegU32(state, 0, 0, math.Float32bits(2.0))
		writeVRegU32(state, 0, 1, math.Float32bits(3.1))

		alu.Run(state)

		Expect(math.Float32frombits(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(
			BeNumerically("~", -1.1, 1e-4))
	})

	It("should run V_MAD_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 449
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, math.Float32bits(10.0))
		writeVRegU32(state, 0, 1, math.Float32bits(20.0))
		writeVRegU32(state, 0, 2, math.Float32bits(30.0))

		alu.Run(state)

		dst := math.Float32frombits(uint32(state.ReadOperand(state.inst.Dst, 0)))
		Expect(dst).To(Equal(float32(230.0)))
	})

	It("should run V_MAD_I32_I24", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 450
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, int32ToBits(-10))
		writeVRegU32(state, 0, 1, int32ToBits(-20))
		writeVRegU32(state, 0, 2, int32ToBits(-50))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0) & 0xffffffff).To(Equal(uint64(150)))
	})

	It("should run V_MAD_U32_U24", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 451
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, 10)
		writeVRegU32(state, 0, 1, 20)
		writeVRegU32(state, 0, 2, 50)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(250)))
	})

	It("should run V_MIN3_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 464
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, math.Float32bits(1))
		writeVRegU32(state, 0, 1, math.Float32bits(2))
		writeVRegU32(state, 0, 2, math.Float32bits(3))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(math.Float32bits(1)))
	})

	It("should run V_MIN3_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 465
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(0))
		writeVRegU32(state, 0, 2, int32ToBits(1))

		alu.Run(state)

		Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(Equal(int32(-1)))
	})

	It("should run V_MIN3_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 466
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 0, 2, 3)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_MAX3_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 467
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, math.Float32bits(1))
		writeVRegU32(state, 0, 1, math.Float32bits(2))
		writeVRegU32(state, 0, 2, math.Float32bits(3))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(math.Float32bits(3)))
	})

	It("should run V_MAX3_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 468
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(0))
		writeVRegU32(state, 0, 2, int32ToBits(1))

		alu.Run(state)

		Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(Equal(int32(1)))
	})

	It("should run V_MAX3_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 469
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 0, 2, 3)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(3)))
	})

	It("should run V_MED3_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 470
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, math.Float32bits(1))
		writeVRegU32(state, 0, 1, math.Float32bits(2))
		writeVRegU32(state, 0, 2, math.Float32bits(3))

		alu.Run(state)

		Expect(uint32(state.ReadOperand(state.inst.Dst, 0))).To(Equal(math.Float32bits(2)))
	})

	It("should run V_MED3_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 471
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, int32ToBits(-1))
		writeVRegU32(state, 0, 1, int32ToBits(0))
		writeVRegU32(state, 0, 2, int32ToBits(1))

		alu.Run(state)

		Expect(asInt32(uint32(state.ReadOperand(state.inst.Dst, 0)))).To(Equal(int32(0)))
	})

	It("should run V_MED3_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 472
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, 1)
		writeVRegU32(state, 0, 1, 2)
		writeVRegU32(state, 0, 2, 3)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(2)))
	})

	It("should run V_MAD_U64_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 488
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Src2 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)
		state.exec = 1

		writeVRegU32(state, 0, 0, 10)
		writeVRegU32(state, 0, 1, 20)
		writeVRegU64(state, 0, 2, 50)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(250)))
	})

	It("should run V_MUL_LO_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 645
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.exec = 0xffffffffffffffff

		for i := 0; i < 64; i++ {
			writeVRegU32(state, i, 0, uint32(i))
			writeVRegU32(state, i, 1, 2)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(state.ReadOperand(state.inst.Dst, i)).To(Equal(uint64(i * 2)))
		}
	})

	It("should run V_MUL_HI_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 646
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 1)
		state.inst.Dst = insts.NewVRegOperand(0, 2, 1)
		state.exec = 1

		writeVRegU32(state, 0, 0, 0x80000000)
		writeVRegU32(state, 0, 1, 2)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(1)))
	})

	It("should run V_LSHLREV_B64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 655
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 2)
		state.exec = 0x1

		writeVRegU32(state, 0, 0, 3)
		writeVRegU64(state, 0, 1, 0x0000000000010000)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x0000000000080000)))
	})

	It("should run V_ASHRREV_I64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 657
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Src1 = insts.NewVRegOperand(0, 1, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 3, 2)
		state.exec = 0x3

		writeVRegU32(state, 0, 0, 4)
		writeVRegU64(state, 0, 1, 0x0000000000010000)
		writeVRegU32(state, 1, 0, 4)
		writeVRegU64(state, 1, 1, 0xffffffff00010000)

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(uint64(0x0000000000001000)))
		Expect(state.ReadOperand(state.inst.Dst, 1)).To(Equal(uint64(0xfffffffff0001000)))
	})

	It("should run V_ADD_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 640
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, math.Float64bits(2.0))
		writeVRegU64(state, 0, 2, math.Float64bits(3.1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(float64(5.1))))
	})

	It("should run V_FMA_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 460
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Src2 = insts.NewVRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 6, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, math.Float64bits(2.0))
		writeVRegU64(state, 0, 2, math.Float64bits(3.1))
		writeVRegU64(state, 0, 4, math.Float64bits(2.5))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(float64(8.7))))
	})

	It("should run V_MUL_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 641
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 4, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, math.Float64bits(2.0))
		writeVRegU64(state, 0, 2, math.Float64bits(3.1))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(float64(6.2))))
	})

	It("should run V_DIV_FMAS_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 483
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Src2 = insts.NewVRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 6, 2)
		state.exec = 0x1
		state.vcc = 1

		writeVRegU64(state, 0, 0, math.Float64bits(2.0))
		writeVRegU64(state, 0, 2, math.Float64bits(1.1))
		writeVRegU64(state, 0, 4, math.Float64bits(4.0))

		alu.Run(state)

		Expect(state.ReadOperand(state.inst.Dst, 0)).
			To(Equal(math.Float64bits(float64(6.2) * math.Pow(2.0, 64))))
	})

	It("should run V_DIV_FIXUP_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 479
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Src2 = insts.NewVRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 6, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, math.Float64bits(0))
		writeVRegU64(state, 0, 2, math.Float64bits(0))
		writeVRegU64(state, 0, 4, math.Float64bits(0))

		alu.Run(state)
		// 0 / 0
		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(0xFFF8000000000000)))
	})

	It("should run V_DIV_FIXUP_F64 inf/inf", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP3a
		state.inst.Opcode = 479
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Src1 = insts.NewVRegOperand(0, 2, 2)
		state.inst.Src2 = insts.NewVRegOperand(0, 4, 2)
		state.inst.Dst = insts.NewVRegOperand(0, 6, 2)
		state.exec = 0x1

		writeVRegU64(state, 0, 0, math.Float64bits(0))
		writeVRegU64(state, 0, 2, math.Float64bits(0x7FF0000000000000))
		writeVRegU64(state, 0, 4, math.Float64bits(0x7FF0000000000000))

		alu.Run(state)
		// inf / inf
		Expect(state.ReadOperand(state.inst.Dst, 0)).To(Equal(math.Float64bits(0xFFF8000000000000)))
	})
})
