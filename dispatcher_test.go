package gcn3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type mockGridBuilder struct {
	Grid *kernels.Grid
}

func (b *mockGridBuilder) Build(hsaco *insts.HsaCo, packet *kernels.HsaKernelDispatchPacket) *kernels.Grid {
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
		engine                 *akita.MockEngine
		grid                   *kernels.Grid
		gridBuilder            *mockGridBuilder
		toCommandProcessorConn *akita.MockConnection
		toCUsConn              *akita.MockConnection

		cu0 *akita.MockComponent
		cu1 *akita.MockComponent
	)

	BeforeEach(func() {
		engine = akita.NewMockEngine()

		grid = prepareGrid()
		gridBuilder = new(mockGridBuilder)
		gridBuilder.Grid = grid

		dispatcher = NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1

		toCommandProcessorConn = akita.NewMockConnection()
		toCommandProcessorConn.PlugIn(dispatcher.ToCommandProcessor)
		toCUsConn = akita.NewMockConnection()
		toCUsConn.PlugIn(dispatcher.ToCUs)

		cu0 = akita.NewMockComponent("cu0")
		cu1 = akita.NewMockComponent("cu1")
		dispatcher.RegisterCU(cu0.ToOutside)
		dispatcher.RegisterCU(cu1.ToOutside)
	})

	It("start kernel launching", func() {
		dispatcher.dispatchingReq = nil

		req := NewLaunchKernelReq(10, nil, dispatcher.ToCommandProcessor)
		req.SetRecvTime(10)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	//It("should reject dispatching if it is dispatching another kernel", func() {
	//	req := NewLaunchKernelReq(5, nil, dispatcher.ToCP)
	//	dispatcher.dispatchingReq = req
	//
	//	anotherReq := NewLaunchKernelReq(10, nil, dispatcher.ToCP)
	//	anotherReq.SetRecvTime(10)
	//
	//	expectedReq := NewLaunchKernelReq(10, dispatcher.ToCP, nil)
	//	expectedReq.OK = false
	//	expectedReq.SetSendTime(10)
	//	expectedReq.SetRecvTime(10)
	//	toCommandProcessorConn.ExpectSend(expectedReq, nil)
	//
	//	dispatcher.Handle(anotherReq)
	//
	//	Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
	//	Expect(len(engine.ScheduledEvent)).To(Equal(0))
	//})

	It("should map work-group", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq

		wg := grid.WorkGroups[0]
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		expectedReq := NewMapWGReq(dispatcher.ToCUs, cu0.ToOutside, 10, wg)
		toCUsConn.ExpectSend(expectedReq, nil)

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(toCUsConn.AllExpectedSent()).To(BeTrue())
	})

	It("should reschedule work-group mapping if sending failed", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq

		wg := grid.WorkGroups[0]
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		expectedReq := NewMapWGReq(dispatcher.ToCUs, cu0.ToOutside, 10, wg)
		toCUsConn.ExpectSend(expectedReq, akita.NewSendError())

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
		dispatcher.cuBusy[cu0.ToOutside] = true
		dispatcher.cuBusy[cu1.ToOutside] = true
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups[0])

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should mark CU busy if MapWGReq failed", func() {
		dispatcher.dispatchingCUID = 0
		wg := grid.WorkGroups[0]
		req := NewMapWGReq(cu0.ToOutside, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)
		req.Ok = false

		dispatcher.Handle(req)

		Expect(dispatcher.cuBusy[cu0.ToOutside]).To(BeTrue())
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should map another work-group when finished mapping a work-group", func() {
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)

		wg := grid.WorkGroups[0]
		req := NewMapWGReq(cu0.ToOutside, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)
		req.Ok = true

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should continue dispatching when receiving WGFinishMesg", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.cuBusy[cu0.ToOutside] = true

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0.ToOutside, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(dispatcher.cuBusy[cu0.ToOutside]).To(BeFalse())
	})

	It("should not continue dispatching when receiving WGFinishMesg and "+
		"the dispatcher is dispatching", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.state = DispatcherToMapWG

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0.ToOutside, dispatcher.ToCUs, 10, wg)

		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should send the KernelLaunchingReq back to the command processor, "+
		"when receiving WGFinishMesg and there is no more work-groups", func() {
		kernelLaunchingReq := NewLaunchKernelReq(10,
			nil, dispatcher.ToCommandProcessor)
		dispatcher.dispatchingReq = kernelLaunchingReq
		dispatcher.dispatchingGrid = grid

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0.ToOutside, dispatcher.ToCUs, 10, wg)

		dispatcher.completedWGs = append(dispatcher.completedWGs,
			grid.WorkGroups[1:]...)

		toCommandProcessorConn.ExpectSend(kernelLaunchingReq, nil)
		dispatcher.Handle(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(0))
		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
		Expect(dispatcher.dispatchingReq).To(BeNil())
	})
})
