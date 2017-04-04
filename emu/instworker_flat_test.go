package emu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
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
		wf = emu.NewWfScheduleInfo()
	})

	It("should run FlatLoadUShort", func() {
		inst := insts.NewInstruction()
		inst.FormatType = insts.Flat
		inst.Opcode = 18
		inst.ByteSize = 4
		inst.Addr = insts.NewVRegOperand(0, 2)
		inst.Dst = insts.NewVRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(emu.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(insts.Regs[insts.Exec], 0, 8,
			insts.Uint64ToBytes(0xffffffffffffffff))
		for i := 0; i < 64; i++ {
			cu.ExpectRegRead(insts.VReg(0), i, 8,
				insts.Uint64ToBytes(uint64(15)))
			req := mem.NewAccessReq()
			info := new(emu.MemAccessInfo)
			req.Info = info
			info.Ready = true
			cu.ExpectReadMem(15, 2, nil, 0, req, nil)
		}
		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8,
			insts.Uint64ToBytes(6000))
		cu.ExpectRegWrite(insts.Regs[insts.Pc], 0, insts.Uint64ToBytes(6004))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()

	})

})
