package emu

import (
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

})
