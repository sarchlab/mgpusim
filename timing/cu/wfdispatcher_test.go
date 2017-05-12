package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

var _ = Describe("WfDispatcher", func() {
	var (
		wfDispatcher *WfDispatcherImpl
		scheduler    *Scheduler
	)

	BeforeEach(func() {
		wfDispatcher = new(WfDispatcherImpl)
		scheduler = NewScheduler("scheduler", nil, nil, wfDispatcher, nil, nil, nil)
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

		evt := NewDispatchWfEvent(0, scheduler, req)

		ok, managedWf := wfDispatcher.DispatchWf(evt)

		Expect(ok).To(BeTrue())
		Expect(scheduler.WfPools[0].Availability()).To(Equal(9))
		Expect(managedWf.PC).To(Equal(uint64(6064)))
		Expect(managedWf.State).To(Equal(WfReady))
	})
})
