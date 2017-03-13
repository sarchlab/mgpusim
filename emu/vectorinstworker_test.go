package emu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("VectorInstWorker", func() {
	var (
		cu *gcn3.MockComputeUnit
		w  *emu.VectorInstWorker
	)

	BeforeEach(func() {
		cu = gcn3.NewMockComputeUnit("mockCU")
		w = emu.NewVectorInstWorker()
		w.CU = cu
	})

	It("should run v_mov_b32", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Vop1
		inst.Opcode = 1
		inst.ByteSize = 4
		inst.Src0 = disasm.NewSRegOperand(0, 1)
		inst.Dst = disasm.NewVRegOperand(2, 1)

		cu.ExpectRegRead(disasm.Regs[disasm.Exec], 0, 8,
			disasm.Uint64ToBytes(0xffffffffffffffff))
		for i := 0; i < 64; i++ {
			cu.ExpectRegRead(disasm.SReg(0), i, 4, disasm.Uint32ToBytes(uint32(15)))
			cu.ExpectRegWrite(disasm.VReg(2), i, disasm.Uint32ToBytes(uint32(15)))
		}
		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(6000))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))

		w.Run(inst, 0)

		cu.AllExpectedAccessed()
	})

})
