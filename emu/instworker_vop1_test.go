package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
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
		inst := insts.NewInstruction()
		inst.FormatType = insts.Vop1
		inst.Opcode = 1
		inst.ByteSize = 4
		inst.Src0 = insts.NewSRegOperand(0, 1)
		inst.Dst = insts.NewVRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(kernels.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(insts.Regs[insts.Exec], 0, 8,
			insts.Uint64ToBytes(0xffffffffffffffff))
		for i := 0; i < 64; i++ {
			cu.ExpectRegRead(insts.SReg(0), i, 4, insts.Uint32ToBytes(uint32(15)))
			cu.ExpectRegWrite(insts.VReg(2), i, insts.Uint32ToBytes(uint32(15)))
		}
		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8,
			insts.Uint64ToBytes(6000))
		cu.ExpectRegWrite(insts.Regs[insts.Pc], 0, insts.Uint64ToBytes(6004))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})

})
