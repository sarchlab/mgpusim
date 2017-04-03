package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("InstWorkerImpl_Sop2", func() {
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

	It("should run v_mov_b32", func() {
		inst := disasm.NewInstruction()
		inst.FormatType = disasm.Vop1
		inst.Opcode = 1
		inst.ByteSize = 4
		inst.Src0 = disasm.NewSRegOperand(0, 1)
		inst.Dst = disasm.NewVRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(emu.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(disasm.Regs[disasm.Exec], 0, 8,
			disasm.Uint64ToBytes(0xffffffffffffffff))
		for i := 0; i < 64; i++ {
			cu.ExpectRegRead(disasm.SReg(0), i, 4, disasm.Uint32ToBytes(uint32(15)))
			cu.ExpectRegWrite(disasm.VReg(2), i, disasm.Uint32ToBytes(uint32(15)))
		}
		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(6000))
		cu.ExpectRegWrite(disasm.Regs[disasm.Pc], 0, disasm.Uint64ToBytes(6004))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})

})
