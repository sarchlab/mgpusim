package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("InstWorkerImpl", func() {
	var (
		cu        *gcn3.MockComputeUnit
		w         *emu.InstWorkerImpl
		wf        *emu.WfScheduleInfo
		scheduler *emu.Scheduler
	)

	BeforeEach(func() {
		cu = gcn3.NewMockComputeUnit("mockCU")
		scheduler = emu.NewScheduler()
		w = new(emu.InstWorkerImpl)
		w.CU = cu
		w.Scheduler = scheduler
		wf = new(emu.WfScheduleInfo)
	})

	It("should run s_add_u32", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Sop2
		inst.Opcode = 0
		inst.ByteSize = 4
		inst.Src0 = disasm.NewSRegOperand(0, 1)
		inst.Src1 = disasm.NewSRegOperand(1, 1)
		inst.Dst = disasm.NewSRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(emu.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(6000))
		cu.ExpectRegRead(disasm.SReg(1), 0, 4, disasm.Uint32ToBytes(uint32(15)))
		cu.ExpectRegRead(disasm.SReg(0), 0, 4, disasm.Uint32ToBytes(uint32(10)))
		cu.ExpectRegWrite(disasm.SReg(2), 0, disasm.Uint32ToBytes(uint32(25)))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))
		cu.ExpectRegWrite(disasm.Regs[disasm.Scc], 0, disasm.Uint8ToBytes(0))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})
})
