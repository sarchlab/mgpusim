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

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(3)))
		Expect(sp.DST[1]).To(Equal(uint64(2)))
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

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(sp.DST[0]))).To(
				Equal(int32(math.MinInt32 + 1)))
		}
		Expect(sp.VCC).To(Equal(uint64(0xffffffffffffffff)))
	})

	It("should run V_ADD_I32, with negtive overflow", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 25

		sp := state.Scratchpad().AsVOP2()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(int32ToBits(math.MinInt32 + 10))
			sp.SRC1[i] = uint64(int32ToBits(-12))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(asInt32(uint32(sp.DST[0]))).To(
				Equal(int32(math.MaxInt32 - 1)))
		}
		Expect(sp.VCC).To(Equal(uint64(0xffffffffffffffff)))
	})

	It("should run V_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 28

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = math.MaxUint32 - 10
		sp.SRC1[0] = 10
		sp.VCC = uint64(1)

		alu.Run(state)

		Expect(uint32(sp.DST[0])).To(Equal(uint32(0)))
		Expect(sp.VCC).To(Equal(uint64(1)))
	})

	It("should run V_MAC_F32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop2
		state.inst.Opcode = 22

		sp := state.Scratchpad().AsVOP2()
		sp.SRC0[0] = uint64(float32ToBits(4))
		sp.SRC1[0] = uint64(float32ToBits(16))
		sp.DST[0] = uint64(float32ToBits(1024))

		alu.Run(state)

		Expect(asFloat32(uint32(sp.DST[0]))).To(Equal(float32(1024.0 + 16.0*4.0)))
	})
})
