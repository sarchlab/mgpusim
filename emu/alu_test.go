package emu

import (
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

type mockInstState struct {
	inst       *insts.Inst
	scratchpad Scratchpad
}

func (s *mockInstState) Inst() *insts.Inst {
	return s.inst
}

func (s *mockInstState) Scratchpad() Scratchpad {
	return s.scratchpad
}

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

	It("should run S_ADD_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 0

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1))   // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31+15)) // SRC1
		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).To(Equal(uint32(14)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run S_ADDC_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sop2
		state.inst.Opcode = 4

		copy(state.scratchpad[0:8], insts.Uint32ToBytes(1<<31-1)) // SRC0
		copy(state.scratchpad[8:16], insts.Uint32ToBytes(1<<31))  // SRC1
		state.scratchpad[24] = 1                                  // SCC

		alu.Run(state)

		Expect(insts.BytesToUint32(state.scratchpad[16:24])).To(Equal(uint32(0)))
		Expect(state.scratchpad[24]).To(Equal(byte(1)))
	})

	It("should run V_MOV_B32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop1
		state.inst.Opcode = 1

		alu.Run(state)

		sp := state.Scratchpad()
		for i := 0; i < 64*8; i++ {
			Expect(sp[i]).To(Equal(sp[i+512]))
		}
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

	It("should run V_MUL_LO_U32", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 645

		sp := state.Scratchpad().AsVOP3A()
		for i := 0; i < 64; i++ {
			sp.SRC0[i] = uint64(i)
			sp.SRC1[i] = uint64(2)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(sp.DST[i]).To(Equal(uint64(i * 2)))
		}
	})

	It("should run V_ASHRREV_I64", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Vop3
		state.inst.Opcode = 657

		sp := state.Scratchpad().AsVOP3A()
		sp.SRC1[0] = uint64(0x0000000000010000)
		sp.SRC1[1] = uint64(0xffffffff00010000)
		sp.SRC0[0] = 4
		sp.SRC0[1] = 4

		alu.Run(state)

		Expect(sp.DST[0]).To(Equal(uint64(0x0000000000001000)))
		Expect(sp.DST[1]).To(Equal(uint64(0xfffffffff0001000)))
	})

	It("should run FLAT_LOAD_USHORT", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 18

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_LOAD_DWROD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 20

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			storage.Write(uint64(i*4), insts.Uint32ToBytes(uint32(i)))
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			Expect(layout.DST[i*4]).To(Equal(uint32(i)))
		}
	})

	It("should run FLAT_STORE_DWORD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Flat
		state.inst.Opcode = 28

		layout := state.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			layout.ADDR[i] = uint64(i * 4)
			layout.DATA[i*4] = uint32(i)
		}

		alu.Run(state)

		for i := 0; i < 64; i++ {
			buf, err := storage.Read(uint64(i*4), uint64(4))
			Expect(err).To(BeNil())
			Expect(insts.BytesToUint32(buf)).To(Equal(uint32(i)))
		}
	})

	It("should run S_LOAD_DWORD", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Smem
		state.inst.Opcode = 0

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
	})

	It("should run S_LOAD_DWORDX2", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Smem
		state.inst.Opcode = 1

		layout := state.Scratchpad().AsSMEM()
		layout.Base = 1024
		layout.Offset = 16

		storage.Write(uint64(1040), insts.Uint32ToBytes(217))
		storage.Write(uint64(1044), insts.Uint32ToBytes(218))

		alu.Run(state)

		Expect(layout.DST[0]).To(Equal(uint32(217)))
		Expect(layout.DST[1]).To(Equal(uint32(218)))
	})

	It("should run S_CMP_EQ_U32 when input is not equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopc
		state.inst.Opcode = 6

		layout := state.Scratchpad().AsSOPC()
		layout.SRC0 = 1
		layout.SRC1 = 2

		alu.Run(state)

		Expect(layout.SCC).To(Equal(byte(0)))
	})

	It("should run S_CMP_EQ_U32 when input is equal", func() {
		state.inst = insts.NewInst()
		state.inst.FormatType = insts.Sopc
		state.inst.Opcode = 6

		layout := state.Scratchpad().AsSOPC()
		layout.SRC0 = 1
		layout.SRC1 = 1

		alu.Run(state)

		Expect(layout.SCC).To(Equal(byte(1)))
	})

})
