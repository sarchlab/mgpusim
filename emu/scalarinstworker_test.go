package emu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("ScalarInstWorker", func() {
	var (
		cu *gcn3.MockComputeUnit
		w  *emu.ScalarInstWorker
	)

	BeforeEach(func() {
		cu = gcn3.NewMockComputeUnit("mockCU")
		w = emu.NewScalarInstWorker()
		w.CU = cu
	})

	It("should run s_add_u32", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Sop2
		inst.Opcode = 0
		inst.ByteSize = 4
		inst.Src0 = disasm.NewSRegOperand(0, 1)
		inst.Src1 = disasm.NewSRegOperand(1, 1)
		inst.Dst = disasm.NewSRegOperand(2, 1)

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(6000))
		cu.ExpectRegRead(disasm.SReg(1), 0, 4, disasm.Uint32ToBytes(uint32(15)))
		cu.ExpectRegRead(disasm.SReg(0), 0, 4, disasm.Uint32ToBytes(uint32(10)))
		cu.ExpectRegWrite(disasm.SReg(2), 0, disasm.Uint32ToBytes(uint32(25)))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))
		cu.ExpectRegWrite(disasm.Regs[disasm.Scc], 0, disasm.Uint8ToBytes(0))

		w.Run(inst, 0)

		cu.AllExpectedAccessed()
	})

	It("should run s_add_u32 with carry", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Sop2
		inst.Opcode = 0
		inst.ByteSize = 4
		inst.Src0 = disasm.NewIntOperand(1 << 31)
		inst.Src1 = disasm.NewIntOperand(1 << 31)
		inst.Dst = disasm.NewSRegOperand(2, 1)

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8, disasm.Uint64ToBytes(6000))
		cu.ExpectRegWrite(disasm.SReg(2), 0, disasm.Uint32ToBytes(uint32(0)))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))
		cu.ExpectRegWrite(disasm.Regs[disasm.Scc], 0, disasm.Uint8ToBytes(1))

		w.Run(inst, 0)

		cu.AllExpectedAccessed()
	})

	It("should run s_addc_u32", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Sop2
		inst.Opcode = 4
		inst.ByteSize = 4
		inst.Src0 = disasm.NewIntOperand(1 << 31)
		inst.Src1 = disasm.NewIntOperand(1 << 31)
		inst.Dst = disasm.NewSRegOperand(2, 1)

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8, disasm.Uint64ToBytes(6000))
		cu.ExpectRegRead(disasm.Regs[disasm.Scc], 0, 1, disasm.Uint8ToBytes(1))
		cu.ExpectRegWrite(disasm.SReg(2), 0, disasm.Uint32ToBytes(uint32(1)))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))
		cu.ExpectRegWrite(disasm.Regs[disasm.Scc], 0, disasm.Uint8ToBytes(1))

		w.Run(inst, 0)

		cu.AllExpectedAccessed()
	})

})
