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

	It("should run V_MOV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 1
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x00000000ffffffff

		for i := 0; i < 32; i++ {
			offset := i*256*4 + 0*4
			copy(state.vRegFile[offset:], insts.Uint32ToBytes(1))
		}

		alu.Run(state)

		for i := 0; i < 32; i++ {
			result := state.ReadOperand(state.inst.Dst, i)
			Expect(result).To(Equal(uint64(1)))
		}

		for i := 32; i < 64; i++ {
			result := state.ReadOperand(state.inst.Dst, i)
			Expect(result).To(Equal(uint64(0)))
		}
	})

	It("should run V_READFIRSTLANE_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 2
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x0000000000000100

		// Set lane 8, v0 = 1
		offset := 8*256*4 + 0*4
		copy(state.vRegFile[offset:], insts.Uint32ToBytes(1))

		alu.Run(state)

		for i := 0; i < 64; i++ {
			result := state.ReadOperand(state.inst.Dst, i)
			Expect(result).To(Equal(uint64(1)))
		}
	})

	It("should run V_CVT_F64_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 4
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x1

		// lane 0, v0 = 1
		copy(state.vRegFile[0:], insts.Uint32ToBytes(1))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float64frombits(result)).To(Equal(float64(1.0)))
	})

	It("should run V_CVT_F32_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 5
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(int32ToBits(-1)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float32frombits(uint32(result))).To(Equal(float32(-1.0)))
	})

	It("should run V_CVT_F32_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 6
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(1))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float32frombits(uint32(result))).To(Equal(float32(1.0)))
	})

	It("should run V_CVT_U32_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(1.0)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(1)))
	})

	It("should run V_CVT_U32_F32, when input is nan", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(float32(math.NaN()))))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(0)))
	})

	It("should run V_CVT_U32_F32, when the input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(-1.0)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(0)))
	})

	It("should run V_CVT_U32_F32, when the input is very large", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(float32(math.MaxUint32+1))))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(math.MaxUint32)))
	})

	It("should run V_CVT_I32_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(1.5)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(1)))
	})

	It("should run V_CVT_I32_F32, when input is nan", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(float32(0-math.NaN()))))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(0)))
	})

	It("should run V_CVT_I32_F32, when the input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(-1.5)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(int32ToBits(-1))))
	})

	It("should run V_CVT_I32_F32, when the input is very large", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(0-float32(math.MaxInt32)-1)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(uint64(int32ToBits(0 - math.MaxInt32))))
	})

	It("should run V_TRUNC_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 28
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		// lane 0: 1.1
		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.1)))
		// lane 1: -2.2
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(-2.2)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(-2.0)))
	})

	It("should run V_RNDNE_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 30
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.1)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(-2.6)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(-3.0)))
	})

	It("should run V_EXP_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 32
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.1)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(-2.6)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).
			To(BeNumerically("~", float32(2.1436), 1e-3))
		Expect(math.Float32frombits(uint32(r1))).
			To(BeNumerically("~", float32(0.1649), 1e-3))
	})

	It("should run V_LOG_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 33
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.1)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(-2.6)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).
			To(BeNumerically("~", float32(0.1375), 1e-3))
		Expect(math.IsNaN(float64(math.Float32frombits(uint32(r1))))).
			To(BeTrue())
	})

	It("should run V_RCP_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 34
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.0)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(0.5)))
	})

	It("should run V_RCP_IFLAG_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 35
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(1.0)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(2.0)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(0.5)))
	})

	It("should run V_RSQ_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 36
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(4.0)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(625.0)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(0.5)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(0.04)))
	})

	It("should run V_SQRT_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 39
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x3

		copy(state.vRegFile[0*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(4.0)))
		copy(state.vRegFile[1*256*4+0*4:], insts.Uint32ToBytes(math.Float32bits(625.0)))

		alu.Run(state)

		r0 := state.ReadOperand(state.inst.Dst, 0)
		r1 := state.ReadOperand(state.inst.Dst, 1)
		Expect(math.Float32frombits(uint32(r0))).To(Equal(float32(2.0)))
		Expect(math.Float32frombits(uint32(r1))).To(Equal(float32(25.0)))
	})

	It("should run V_CVT_F32_UBYTE0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 17
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(256.0)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float32frombits(uint32(result))).To(Equal(float32(0)))
	})

	It("should run V_CVT_F64_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 16
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(-1.0)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(result).To(Equal(math.Float64bits(float64(-1.0))))
	})

	It("should run V_RCP_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 37
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 2)
		state.exec = 0x1

		// Write 64-bit float 25.0 to lane 0, v0-v1
		bits := math.Float64bits(25.0)
		copy(state.vRegFile[0:], insts.Uint64ToBytes(bits))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float64frombits(result)).To(Equal(float64(0.04)))
	})

	It("should run V_CVT_F32_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 15
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		state.inst.Dst = insts.NewVRegOperand(2, 2, 1)
		state.exec = 0x1

		bits := math.Float64bits(25.0)
		copy(state.vRegFile[0:], insts.Uint64ToBytes(bits))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(math.Float32frombits(uint32(result))).To(Equal(float32(25.0)))
	})

	It("should run V_CVT_F16_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 10
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(math.Float32bits(8.0)))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		// value 8.0 => half - precision : 0x4800
		Expect(uint16(result)).To(Equal(uint16(0x4800)))
	})

	It("should run V_BREV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 44
		state.inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		state.inst.Dst = insts.NewVRegOperand(1, 1, 1)
		state.exec = 0x1

		copy(state.vRegFile[0:], insts.Uint32ToBytes(0xffff))

		alu.Run(state)

		result := state.ReadOperand(state.inst.Dst, 0)
		Expect(uint32(result)).To(Equal(uint32(0xffff0000)))
	})

})
