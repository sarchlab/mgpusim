package gcn3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type mockGridBuilder struct {
	Grid *kernels.Grid
}

func (b *mockGridBuilder) Build(req *kernels.LaunchKernelReq) *kernels.Grid {
	return b.Grid
}

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

var _ = Describe("Dispatcher", func() {
	var (
		dispatcher             *Dispatcher
		engine                 *core.MockEngine
		grid                   *kernels.Grid
		gridBuilder            *mockGridBuilder
		toCommandProcessorConn *core.MockConnection
		toCUsConn              *core.MockConnection

		cu0 *core.MockComponent
		cu1 *core.MockComponent
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()

		grid = prepareGrid()
		gridBuilder = new(mockGridBuilder)
		gridBuilder.Grid = grid

		dispatcher = NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1

		toCommandProcessorConn = core.NewMockConnection()
		core.PlugIn(dispatcher, "ToCommandProcessor", toCommandProcessorConn)
		toCUsConn = core.NewMockConnection()
		core.PlugIn(dispatcher, "ToCUs", toCUsConn)

		cu0 = core.NewMockComponent("cu0")
		cu1 = core.NewMockComponent("cu1")
		dispatcher.RegisterCU(cu0)
		dispatcher.RegisterCU(cu1)
	})

	It("start kernel launching", func() {
		dispatcher.dispatchingReq = nil

		req := kernels.NewLaunchKernelReq()
		req.SetSrc(nil)
		req.SetDst(dispatcher)
		req.SetRecvTime(10)

		expectedReq := kernels.NewLaunchKernelReq()
		expectedReq.OK = true
		expectedReq.SetSrc(dispatcher)
		expectedReq.SetDst(nil)
		expectedReq.SetSendTime(10)
		expectedReq.SetRecvTime(10)
		toCommandProcessorConn.ExpectSend(expectedReq, nil)

		dispatcher.Handle(req)

		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should reject dispatching if it is dispatching another kernel", func() {
		req := kernels.NewLaunchKernelReq()
		dispatcher.dispatchingReq = req

		anotherReq := kernels.NewLaunchKernelReq()
		anotherReq.SetSrc(nil)
		anotherReq.SetDst(dispatcher)
		anotherReq.SetRecvTime(10)

		expectedReq := kernels.NewLaunchKernelReq()
		expectedReq.OK = false
		expectedReq.SetSrc(dispatcher)
		expectedReq.SetDst(nil)
		expectedReq.SetSendTime(10)
		expectedReq.SetRecvTime(10)
		toCommandProcessorConn.ExpectSend(expectedReq, nil)

		dispatcher.Handle(anotherReq)

		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should map work-group", func() {
		wg := grid.WorkGroups[0]
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		expectedReq := NewMapWGReq(dispatcher, cu0, 10, wg)
		toCUsConn.ExpectSend(expectedReq, nil)

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
	})

	It("should reschedule work-group mapping if sending failed", func() {
		wg := grid.WorkGroups[0]
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		expectedReq := NewMapWGReq(dispatcher, cu0, 10, wg)
		toCUsConn.ExpectSend(expectedReq,
			core.NewError("busy", true, 12))

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should do nothing if all work-groups are mapped", func() {
		dispatcher.dispatchingCUID = -1

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should do nothing if all cus are busy", func() {
		dispatcher.cuBusy[cu0] = true
		dispatcher.cuBusy[cu1] = true

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should mark CU busy if MapWGReq failed", func() {
		dispatcher.dispatchingCUID = 0
		wg := grid.WorkGroups[0]
		req := NewMapWGReq(cu0, dispatcher, 10, wg)
		req.Ok = false

		dispatcher.Handle(req)

		Expect(dispatcher.cuBusy[cu0]).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should start dispatching wavefronts after successfully mapping"+
		" a work-group", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)

		wg := grid.WorkGroups[0]
		req := NewMapWGReq(cu0, dispatcher, 10, wg)
		req.Ok = true

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should dispatch wavefront", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWfs = append(dispatcher.dispatchingWfs,
			grid.WorkGroups[0].Wavefronts...)
		wf := dispatcher.dispatchingWfs[0]

		evt := NewDispatchWfEvent(10, dispatcher)

		expectedReq := NewDispatchWfReq(dispatcher, cu0, 10, wf)
		toCUsConn.ExpectSend(expectedReq, nil)

		dispatcher.Handle(evt)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
	})

	It("should reschedule wavefront dispatching upon error on network", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWfs = append(dispatcher.dispatchingWfs,
			grid.WorkGroups[0].Wavefronts...)
		wf := dispatcher.dispatchingWfs[0]

		evt := NewDispatchWfEvent(10, dispatcher)

		expectedReq := NewDispatchWfReq(dispatcher, cu0, 10, wf)
		toCUsConn.ExpectSend(expectedReq,
			core.NewError("busy", true, 12))

		dispatcher.Handle(evt)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should continue dispatching wavefronts after dispatching another one", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWfs = append(dispatcher.dispatchingWfs,
			grid.WorkGroups[0].Wavefronts...)
		wf := dispatcher.dispatchingWfs[0]
		wf2 := dispatcher.dispatchingWfs[1]

		req := NewDispatchWfReq(cu0, dispatcher, 10, wf)
		req.SetRecvTime(10)

		expectedReq := NewDispatchWfReq(dispatcher, cu0, 10, wf2)
		toCUsConn.ExpectSend(expectedReq, nil)

		dispatcher.Handle(req)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
		Expect(len(dispatcher.dispatchingWfs)).To(
			Equal(len(grid.WorkGroups[0].Wavefronts) - 1))
	})

	It("should retry dispatching wavefronts upon error on network", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWfs = append(dispatcher.dispatchingWfs,
			grid.WorkGroups[0].Wavefronts...)
		wf := dispatcher.dispatchingWfs[0]
		wf2 := dispatcher.dispatchingWfs[1]

		req := NewDispatchWfReq(cu0, dispatcher, 10, wf)
		req.SetRecvTime(10)

		expectedReq := NewDispatchWfReq(dispatcher, cu0, 10, wf2)
		toCUsConn.ExpectSend(expectedReq,
			core.NewError("busy", true, 12))

		dispatcher.Handle(req)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
		Expect(len(dispatcher.dispatchingWfs)).To(
			Equal(len(grid.WorkGroups[0].Wavefronts) - 1))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should map next workgroup is all wavefronts are dispatched", func() {
		dispatcher.dispatchingCUID = 0
		wf := grid.WorkGroups[0].Wavefronts[0]
		dispatcher.dispatchingWfs = []*kernels.Wavefront{wf}

		req := NewDispatchWfReq(cu0, dispatcher, 10, wf)
		req.SetRecvTime(10)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should continue dispatching when receiving WGFinishMesg", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.cuBusy[cu0] = true

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher, 10, wg)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(dispatcher.cuBusy[cu0]).To(BeFalse())
	})

	It("should not continue dispatching when receiving WGFinishMesg and "+
		"the dispatcher is dispatching", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.hasPendingEvent = true

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher, 10, wg)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should send the KernelLaunchingReq back to the command processor, "+
		"when receiving WGFinishMesg and there is no more work-groups", func() {
		kernelLaunchingReq := kernels.NewLaunchKernelReq()
		dispatcher.dispatchingReq = kernelLaunchingReq
		dispatcher.dispatchingGrid = grid

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher, 10, wg)

		dispatcher.completedWGs = append(dispatcher.completedWGs,
			grid.WorkGroups[1:]...)

		toCommandProcessorConn.ExpectSend(kernelLaunchingReq, nil)
		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
		Expect(dispatcher.dispatchingReq).To(BeNil())
	})

	//
	//It("should process WGFinishMesg", func() {
	//	status := NewKernelDispatchStatus()
	//	status.Grid = grid
	//	status.cuBusy = make([]bool, 4)
	//	status.cuBusy[0] = true
	//	dispatcher.dispatchingKernel = status
	//
	//	req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])
	//	req.CUID = 0
	//
	//	dispatcher.Recv(req)
	//
	//	Expect(status.CompletedWGs).To(ContainElement(grid.WorkGroups[0]))
	//	Expect(status.cuBusy[0]).To(BeFalse())
	//	Expect(engine.ScheduledEvent).NotTo(BeEmpty())
	//})
	//
	//It("should not scheduler tick, if dispatcher is running, when processing "+
	//	"WGFinishMesg", func() {
	//	status := NewKernelDispatchStatus()
	//	status.Grid = grid
	//	status.cuBusy = make([]bool, 4)
	//	status.cuBusy[0] = true
	//	dispatcher.dispatchingKernel = status
	//	dispatcher.running = true
	//
	//	req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])
	//	req.CUID = 0
	//
	//	dispatcher.Recv(req)
	//
	//	Expect(status.CompletedWGs).To(ContainElement(grid.WorkGroups[0]))
	//	Expect(status.cuBusy[0]).To(BeFalse())
	//	Expect(engine.ScheduledEvent).To(BeEmpty())
	//})
	//
	//It("should send back the LaunchKernelReq to the driver", func() {
	//	launchReq := kernels.NewLaunchKernelReq()
	//	launchReq.SetSrc(nil)
	//	launchReq.SetDst(dispatcher)
	//
	//	status := NewKernelDispatchStatus()
	//	status.Grid = grid
	//	status.CompletedWGs = append(status.CompletedWGs,
	//		status.Grid.WorkGroups[1:]...)
	//	status.Req = launchReq
	//	dispatcher.dispatchingKernel = status
	//
	//	req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])
	//
	//	connection.ExpectSend(launchReq, nil)
	//
	//	dispatcher.Recv(req)
	//
	//	Expect(connection.AllExpectedSent()).To(BeTrue())
	//})

})
