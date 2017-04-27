package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
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
		co         *insts.HsaCo
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
		co = insts.NewHsaCo()
		status.CodeObject = co
	})

	Context("when processing MapWGReq", func() {
		It("should process MapWGReq", func() {
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
			info := new(timing.WfDispatchInfo)
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)

			scheduler.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		})
	})

	Context("when handling MapWGEvent", func() {

		It("should send NACK if too many Wavefronts", func() {
			// Each SIMD is running 8 wf in each SIMD. 8 more wfs can handle.
			for i := 0; i < 4; i++ {
				scheduler.WfPoolFreeCount[i] = 2
			}

			req := timing.NewMapWGReq(nil, scheduler, 0, grid.WorkGroups[0],
				status)
			evt := cu.NewMapWGEvent(scheduler, 0, req)

			connection.ExpectSend(req, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())

		})

		It("should send NACK to the dispatcher if too many SReg", func() {
			// 128 groups in total, 125 groups occupied.
			// 3 groups are free -> 48 registers available
			scheduler.SGprMask.SetStatus(0, 125, cu.AllocStatusReserved)

			// 10 Wfs, 64 SGPRs per wf. That is 640 in total
			co.WFSgprCount = 64
			req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
				status)
			evt := cu.NewMapWGEvent(scheduler, 10, req)

			connection.ExpectSend(req, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())
		})

		It("should send NACK to the dispatcher if too large LDS", func() {
			// 240 units occupied, 16 units left -> 4096 Bytes available
			scheduler.LDSMask.SetStatus(0, 240, cu.AllocStatusReserved)

			co.WGGroupSegmentByteSize = 8192
			req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
				status)
			evt := cu.NewMapWGEvent(scheduler, 10, req)

			connection.ExpectSend(req, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())
		})

		It("should send NACK if too many VGPRs", func() {
			// 64 units occupied, 4 units available, 4 * 4 = 16 units
			scheduler.VGprMask[0].SetStatus(0, 60, cu.AllocStatusReserved)
			scheduler.VGprMask[1].SetStatus(0, 60, cu.AllocStatusReserved)
			scheduler.VGprMask[2].SetStatus(0, 60, cu.AllocStatusReserved)
			scheduler.VGprMask[3].SetStatus(0, 60, cu.AllocStatusReserved)

			co.WIVgprCount = 20

			req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
				status)
			evt := cu.NewMapWGEvent(scheduler, 10, req)

			connection.ExpectSend(req, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())
		})

		// It("should send NACK if not all Wavefront can fit the VGPRs requirement", func() {
		// 	// SIMD 0 and 1 do not have enouth VGPRs
		// 	scheduler.VGprFreeCount[0] = 3200
		// 	scheduler.VGprFreeCount[1] = 3200
		// 	scheduler.WfPoolFreeCount[2] = 2
		// 	scheduler.WfPoolFreeCount[3] = 2

		// 	co.WIVgprCount = 102
		// 	req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
		// 		status)
		// 	evt := cu.NewMapWGEvent(scheduler, 10, req)

		// 	connection.ExpectSend(req, nil)

		// 	scheduler.Handle(evt)

		// 	Expect(connection.AllExpectedSent()).To(BeTrue())
		// 	Expect(req.Ok).To(BeFalse())
		// })

		// It("should reserve resources and send ACK back if all requirement satisfy", func() {
		// 	co.WIVgprCount = 20
		// 	co.WFSgprCount = 15
		// 	co.WGGroupSegmentByteSize = 1024

		// 	req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
		// 		status)
		// 	evt := cu.NewMapWGEvent(scheduler, 10, req)

		// 	connection.ExpectSend(req, nil)

		// 	scheduler.Handle(evt)

		// 	Expect(connection.AllExpectedSent()).To(BeTrue())
		// 	Expect(req.Ok).To(BeTrue())
		// 	Expect(scheduler.SGprFreeCount).To(Equal(2048 - 150))
		// 	Expect(scheduler.LDSFreeCount).To(Equal(64*1024 - 1024))
		// 	Expect(scheduler.VGprFreeCount[0]).To(Equal(16384 - 64*3*20))
		// 	Expect(scheduler.VGprFreeCount[1]).To(Equal(16384 - 64*3*20))
		// 	Expect(scheduler.VGprFreeCount[2]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.VGprFreeCount[3]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.WfPoolFreeCount[0]).To(Equal(7))
		// 	Expect(scheduler.WfPoolFreeCount[1]).To(Equal(7))
		// 	Expect(scheduler.WfPoolFreeCount[2]).To(Equal(8))
		// 	Expect(scheduler.WfPoolFreeCount[3]).To(Equal(8))
		// })

		// It("should support non-standard CU size", func() {
		// 	scheduler.SetWfPoolSize(5, []int{10, 10, 8, 8, 8})

		// 	co.WIVgprCount = 20

		// 	req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
		// 		status)
		// 	evt := cu.NewMapWGEvent(scheduler, 10, req)

		// 	connection.ExpectSend(req, nil)

		// 	scheduler.Handle(evt)

		// 	Expect(connection.AllExpectedSent()).To(BeTrue())
		// 	Expect(req.Ok).To(BeTrue())
		// 	Expect(scheduler.WfPoolFreeCount[0]).To(Equal(8))
		// 	Expect(scheduler.WfPoolFreeCount[1]).To(Equal(8))
		// 	Expect(scheduler.WfPoolFreeCount[2]).To(Equal(6))
		// 	Expect(scheduler.WfPoolFreeCount[3]).To(Equal(6))
		// 	Expect(scheduler.WfPoolFreeCount[4]).To(Equal(6))
		// 	Expect(scheduler.VGprFreeCount[0]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.VGprFreeCount[1]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.VGprFreeCount[2]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.VGprFreeCount[3]).To(Equal(16384 - 64*2*20))
		// 	Expect(scheduler.VGprFreeCount[4]).To(Equal(16384 - 64*2*20))

		// })
	})

	Context("when handling dispatch wavefront request", func() {
		It("should handle wavefront diapatch", func() {
			wf := grid.WorkGroups[0].Wavefronts[0]
			info := new(timing.WfDispatchInfo)
			info.SIMDID = 1
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)
			evt := cu.NewDispatchWfEvent(scheduler, 10, req)

			scheduler.Handle(evt)

			Expect(scheduler.Running).To(BeTrue())
			Expect(scheduler.WfPools[1].Wfs).NotTo(BeEmpty())
			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
			Expect(scheduler.WfPools[1].Wfs[0].PC).To(Equal(uint64(6256)))
		})
	})
})
