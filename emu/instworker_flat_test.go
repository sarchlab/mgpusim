package emu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3"
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

	It("should run FlatLoadUShort", func() {

	})

})
