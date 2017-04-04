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

	It("should run s_add_u32", func() {
		inst := insts.NewInstruction()
		inst.FormatType = insts.Sop2
		inst.Opcode = 0
		inst.ByteSize = 4
		inst.Src0 = insts.NewSRegOperand(0, 1)
		inst.Src1 = insts.NewSRegOperand(1, 1)
		inst.Dst = insts.NewSRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(kernels.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8,
			insts.Uint64ToBytes(6000))
		cu.ExpectRegRead(insts.SReg(1), 0, 4, insts.Uint32ToBytes(uint32(15)))
		cu.ExpectRegRead(insts.SReg(0), 0, 4, insts.Uint32ToBytes(uint32(10)))
		cu.ExpectRegWrite(insts.SReg(2), 0, insts.Uint32ToBytes(uint32(25)))
		cu.ExpectRegWrite(insts.Regs[insts.Pc], 0, insts.Uint64ToBytes(6004))
		cu.ExpectRegWrite(insts.Regs[insts.Scc], 0, insts.Uint8ToBytes(0))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})

	It("should run s_add_u32 with carry", func() {
		inst := insts.NewInstruction()
		inst.FormatType = insts.Sop2
		inst.Opcode = 0
		inst.ByteSize = 4
		inst.Src0 = insts.NewIntOperand(1 << 31)
		inst.Src1 = insts.NewIntOperand(1 << 31)
		inst.Dst = insts.NewSRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(kernels.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8, insts.Uint64ToBytes(6000))
		cu.ExpectRegWrite(insts.SReg(2), 0, insts.Uint32ToBytes(uint32(0)))
		cu.ExpectRegWrite(insts.Regs[insts.Pc], 0, insts.Uint64ToBytes(6004))
		cu.ExpectRegWrite(insts.Regs[insts.Scc], 0, insts.Uint8ToBytes(1))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})

	It("should run s_addc_u32", func() {
		inst := insts.NewInstruction()
		inst.FormatType = insts.Sop2
		inst.Opcode = 4
		inst.ByteSize = 4
		inst.Src0 = insts.NewIntOperand(1 << 31)
		inst.Src1 = insts.NewIntOperand(1 << 31)
		inst.Dst = insts.NewSRegOperand(2, 1)

		wf.Inst = inst
		wf.Wf = new(kernels.Wavefront)
		wf.Wf.FirstWiFlatID = 0

		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8, insts.Uint64ToBytes(6000))
		cu.ExpectRegRead(insts.Regs[insts.Scc], 0, 1, insts.Uint8ToBytes(1))
		cu.ExpectRegWrite(insts.SReg(2), 0, insts.Uint32ToBytes(uint32(1)))
		cu.ExpectRegWrite(insts.Regs[insts.Pc], 0, insts.Uint64ToBytes(6004))
		cu.ExpectRegWrite(insts.Regs[insts.Scc], 0, insts.Uint8ToBytes(1))

		w.Run(wf, 0)

		cu.AllExpectedAccessed()
		Expect(wf.State).To(Equal(emu.Ready))
	})
})
