package emu_test

import (
	"github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = ginkgo.Describe("Schedule", func() {
	var (
		scheduler    *emu.Scheduler
		cu           *gcn3.MockComputeUnit
		disassembler *disasm.Disassembler
	)

	ginkgo.BeforeEach(func() {
		scheduler = emu.NewScheduler()
		disassembler = disasm.NewDisassembler()
		cu = gcn3.NewMockComputeUnit("cu")
		scheduler.CU = cu
		scheduler.Decoder = disassembler
	})

	ginkgo.It("should schedule fetch", func() {
		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(4000))
		cu.ExpectReadInstMem(4000, 8, nil, 0)

		scheduler.Schedule(0)

		cu.AllExpectedAccessed()
	})
})
