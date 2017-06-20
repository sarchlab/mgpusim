package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

type mockInstState struct {
	inst       *insts.Inst
	scratchpad []byte
}

func (s *mockInstState) Inst() *insts.Inst {
	return s.inst
}

func (s *mockInstState) Scratchpad() []byte {
	return s.scratchpad
}

var _ = Describe("ALU", func() {

	var (
		alu   *ALU
		state *mockInstState
	)

	BeforeEach(func() {
		alu = new(ALU)
		state = new(mockInstState)
		state.scratchpad = make([]byte, 32)
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

})
