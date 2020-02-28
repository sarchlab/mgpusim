package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/insts"
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

	It("should run V_MOV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 1

		sp := state.Scratchpad().AsVOP1()
		for i := 0; i < 32; i++ {
			sp.SRC0[i] = 1
		}
		sp.EXEC = 0x00000000ffffffff

		alu.Run(state)

		for i := 0; i < 32; i++ {
			Expect(sp.SRC0[i]).To(Equal(sp.DST[i]))
		}

		for i := 32; i < 64; i++ {
			Expect(sp.SRC0[i]).To(Equal(uint64(0)))
		}
	})

	It("should run V_READFIRSTLANE_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 2

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[8] = 1
		sp.EXEC = 0x0000000000000100

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(sp.SRC0[8]).To(Equal(sp.DST[i]))
		}

	})

	It("should run V_CVT_F32_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 6

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = 1
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(1.0)))
	})

	It("should run V_CVT_U32_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(1.0))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(1)))
	})

	It("should run V_CVT_U32_F32, when input is nan", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(float32(math.NaN())))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0)))
	})

	It("should run V_CVT_U32_F32, when the input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(-1.0))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0)))
	})

	It("should run V_CVT_U32_F32, when the input is very large", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 7

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(float32(math.MaxUint32 + 1)))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(math.MaxUint32)))
	})

	It("should run V_CVT_I32_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(1.5))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(1)))
	})

	It("should run V_CVT_I32_F32, when input is nan", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(float32(0 - math.NaN())))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0)))
	})

	It("should run V_CVT_I32_F32, when the input is negative", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(-1.5))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(int32ToBits(-1))))
	})

	It("should run V_CVT_I32_F32, when the input is very large", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 8

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(0 - float32(math.MaxInt32) - 1))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(int32ToBits(0 - math.MaxInt32))))
	})

	It("should run V_TRUNC_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 28

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(1.1))
		sp.SRC0[1] = uint64(math.Float32bits(-2.2))
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(sp.DST[1]))).To(Equal(float32(-2.0)))
	})

	It("should run V_RCP_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 34

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(1.0))
		sp.SRC0[1] = uint64(math.Float32bits(2.0))
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(sp.DST[1]))).To(Equal(float32(0.5)))
	})

	It("should run V_RCP_IFLAG_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 35

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(1.0))
		sp.SRC0[1] = uint64(math.Float32bits(2.0))
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(1.0)))
		Expect(math.Float32frombits(uint32(sp.DST[1]))).To(Equal(float32(0.5)))
	})

	It("should run V_RSQ_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 36

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(4.0))
		sp.SRC0[1] = uint64(math.Float32bits(625.0))
		sp.EXEC = 0x3

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(0.5)))
		Expect(math.Float32frombits(uint32(sp.DST[1]))).To(Equal(float32(0.04)))
	})

	It("should run V_CVT_F32_UBYTE0", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 17

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(256.0))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(0)))
	})

	It("should run V_CVT_F64_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 16

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(-1.0))
		sp.EXEC = 0x1

		alu.Run(state)
		Expect(sp.DST[0]).To(Equal(math.Float64bits(float64(-1.0))))
	})

	It("should run V_RCP_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 37

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = math.Float64bits(25.0)
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float64frombits(sp.DST[0])).To(Equal(float64(0.04)))
	})

	It("should run V_CVT_F32_F64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 15

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = math.Float64bits(25.0)
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(Equal(float32(25.0)))
	})

	It("should run V_CVT_F16_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.VOP1
		state.inst.Opcode = 10

		sp := state.Scratchpad().AsVOP1()
		sp.SRC0[0] = uint64(math.Float32bits(8.0))
		sp.EXEC = 0x1

		alu.Run(state)
		// value 8.0 => half - precision : 0x4800
		Expect(uint16(sp.DST[0])).To(Equal(uint16(0x4800)))
	})

})
