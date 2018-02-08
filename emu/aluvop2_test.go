package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

var _ = Describe("ALU", func() {

	var (
		alu     *ALU
		state   *mockInstState
		storage *mem.Storage
	)

	BeforeEach(func() {
		storage = mem.NewStorage(1 * mem.GB)
		alu = new(ALU)
		alu.Storage = storage

		state = new(mockInstState)
		state.scratchpad = make([]byte, 4096)
	})

	It("should run V_CNDMASK_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 0

		sp := state.Scratchpad().AsVOP2()
		sp.VCC = 1
		sp.SRC0[0] = 1
		sp.SRC0[1] = 2
		sp.SRC1[0] = 3
		sp.SRC1[1] = 4
		sp.EXEC = 3

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(3)))
		Expect(sp.DST[1]).To(Equal(uint64(2)))
	})

	It("should run V_ADD_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 1

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(math.Float32bits(2.0))
		sp.SRC1[0] = uint64(math.Float32bits(3.1))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(math.Float32bits(float32(5.1)))))
	})

	It("should run V_SUB_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 2

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(math.Float32bits(2.0))
		sp.SRC1[0] = uint64(math.Float32bits(3.1))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(
			BeNumerically("~", -1.1, 1e-4))
	})

	It("should run V_SUBREV_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 3

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(math.Float32bits(2.0))
		sp.SRC1[0] = uint64(math.Float32bits(3.1))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(math.Float32frombits(uint32(sp.DST[0]))).To(
			BeNumerically("~", 1.1, 1e-4))
	})

	It("should run V_MUL_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 5

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(math.Float32bits(2.0))
		sp.SRC1[0] = uint64(math.Float32bits(3.1))
		sp.EXEC = 0x1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(math.Float32bits(float32(6.2)))))
	})

	It("should run V_AND_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 19

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 2 // 10
		sp.SRC1[0] = 3 // 11
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(2)))
	})

	It("should run V_OR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 20

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 2 // 10
		sp.SRC1[0] = 3 // 11
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(3)))
	})

	It("should run V_XOR_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 21

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 2 // 10
		sp.SRC1[0] = 3 // 11
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(1)))
	})

	It("should run V_MAC_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 22

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(float32ToBits(4))
		sp.SRC1[0] = uint64(float32ToBits(16))
		sp.DST[0] = uint64(float32ToBits(1024))
		sp.EXEC = 1

		alu.Run(state)

		Expect(asFloat32(uint32(sp.DST[0]))).To(Equal(float32(1024.0 + 16.0*4.0)))
	})

	It("should run V_ADD_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 25

		sp := state.Scratchpad().AsVOP2()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(int32ToBits(-100))
			sp.SRC1[i] = uint64(int32ToBits(10))
		}
		sp.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(sp.DST[0]))).To(Equal(int32(-90)))
		}
	})

	It("should run V_ADD_I32, with positive overflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 25

		sp := state.Scratchpad().AsVOP2()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(int32ToBits(math.MaxInt32 - 10))
			sp.SRC1[i] = uint64(int32ToBits(12))
		}
		sp.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(sp.DST[0]))).To(
				Equal(int32(math.MinInt32 + 1)))
		}
		Expect(sp.VCC).To(Equal(uint64(0xffffffffffffffff)))
	})

	It("should run V_ADD_I32, with negative overflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 25

		sp := state.Scratchpad().AsVOP2()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(int32ToBits(math.MinInt32 + 10))
			sp.SRC1[i] = uint64(int32ToBits(-12))
		}
		sp.EXEC = 0xffffffffffffffff

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(sp.DST[0]))).To(
				Equal(int32(math.MaxInt32 - 1)))
		}
		Expect(sp.VCC).To(Equal(uint64(0xffffffffffffffff)))
	})

	It("should run V_SUB_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 26

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 10
		sp.SRC1[0] = 4
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(6)))
		Expect(sp.VCC).To(Equal(uint64(0)))
	})

	It("should run V_SUB_I32, when underflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 26

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 4
		sp.SRC1[0] = 10
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(0xfffffffa)))
		Expect(sp.VCC).To(Equal(uint64(1)))
	})

	It("should run V_SUBREV_I32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 27

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 4
		sp.SRC1[0] = 10
		sp.EXEC = 1

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(6)))
		Expect(sp.VCC).To(Equal(uint64(0)))
	})

	It("should run V_SUBREV_I32, when underflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 27

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = 10
		sp.SRC1[0] = 4
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(0xfffffffa)))
		Expect(sp.VCC).To(Equal(uint64(1)))
	})

	It("should run V_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 28

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = math.MaxUint32 - 10
		sp.SRC1[0] = 10
		sp.VCC = uint64(1)
		sp.EXEC = 1

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(0)))
		Expect(sp.VCC).To(Equal(uint64(1)))
	})

})
