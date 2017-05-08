package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

var _ = Describe("WfDispatcher", func() {
	var (
		wfDispatcher *cu.WfDispatcherImpl
		scheduler    *cu.Scheduler
	)

	BeforeEach(func() {
		wfDispatcher = new(cu.WfDispatcherImpl)
		scheduler = cu.NewScheduler("scheduler", nil, nil, wfDispatcher)
		wfDispatcher.Scheduler = scheduler
	})

	It("should dispatch wavefront", func() {
		wf := kernels.NewWavefront()
		info := new(timing.WfDispatchInfo)
		co := insts.NewHsaCo()
		packet := new(kernels.HsaKernelDispatchPacket)
		req := timing.NewDispatchWfReq(nil, scheduler, 0, wf, info, 6064)
		req.CodeObject = co
		req.Packet = packet

		evt := cu.NewDispatchWfEvent(scheduler, 0, req)

		ok := wfDispatcher.DispatchWf(evt)

		Expect(ok).To(BeTrue())

		Expect(len(scheduler.WfPools[0].Wfs)).To(Equal(1))
		managedWf := scheduler.WfPools[0].Wfs[0]
		Expect(managedWf.PC).To(Equal(uint64(6064)))
		Expect(managedWf.Status).To(Equal(cu.Ready))
	})
})
