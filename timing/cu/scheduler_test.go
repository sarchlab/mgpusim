package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

func prepareGrid() *kernels.Grid {
	// Prepare a mock grid that is expanded
	grid := kernels.NewGrid()
	for i := 0; i < 5; i++ {
		wg := kernels.NewWorkGroup()
		grid.WorkGroups = append(grid.WorkGroups, wg)
		for j := 0; j < 10; j++ {
			wf := kernels.NewWavefront()
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
	}
	return grid
}

var _ = Describe("Scheduler", func() {
	var (
		scheduler  *cu.Scheduler
		connection *core.MockConnection
		engine     *core.MockEngine
		grid       *kernels.Grid
		status     *timing.KernelDispatchStatus
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		scheduler = cu.NewScheduler("scheduler", engine)
		scheduler.Freq = 1 * core.GHz
		connection = core.NewMockConnection()
		core.PlugIn(scheduler, "ToDispatcher", connection)

		grid = prepareGrid()
		status = timing.NewKernelDispatchStatus()
		status.Grid = grid
	})

	Context("when processing MapWGReq", func() {
		It("should map wg", func() {
			wg := kernels.NewWorkGroup()
			status := timing.NewKernelDispatchStatus()
			req := timing.NewMapWGReq(nil, scheduler, 10, wg, status)

			scheduler.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		})
	})

	Context("when processing DispatchWfReq", func() {
		It("should schedule DispatchWfEvent", func() {
			wg := grid.WorkGroups[0]
			wf := wg.Wavefronts[0]
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf)

			scheduler.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		})
	})

	Context("when handling MapWGEvent", func() {
		It("shoule send ACK to dispatcher", func() {
			req := timing.NewMapWGReq(nil, scheduler, 10,
				grid.WorkGroups[0], status)
			evt := cu.NewMapWGEvent(scheduler, 10, req)

			connection.ExpectSend(req, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeTrue())
			Expect(scheduler.NumWfsCanHandle).To(Equal(30))
		})

		It("shoule send NACK to dispatcher, if too many wavefronts", func() {
			req := timing.NewMapWGReq(nil, scheduler, 10,
				grid.WorkGroups[0], status)
			evt := cu.NewMapWGEvent(scheduler, 10, req)

			connection.ExpectSend(req, nil)
			scheduler.NumWfsCanHandle = 8

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())
			Expect(scheduler.NumWfsCanHandle).To(Equal(8))
		})

	})
})
