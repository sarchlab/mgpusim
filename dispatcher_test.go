package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
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
		mockCtrl    *gomock.Controller
		dispatcher  *Dispatcher
		engine      *mock_akita.MockEngine
		grid        *kernels.Grid
		gridBuilder *mockGridBuilder

		toCommandProcessor *mock_akita.MockPort
		toCUs              *mock_akita.MockPort
		cu0                *mock_akita.MockPort
		cu1                *mock_akita.MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)

		grid = prepareGrid()
		gridBuilder = new(mockGridBuilder)
		gridBuilder.Grid = grid

		toCommandProcessor = mock_akita.NewMockPort(mockCtrl)
		toCUs = mock_akita.NewMockPort(mockCtrl)

		dispatcher = NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1
		dispatcher.ToCUs = toCUs
		dispatcher.ToCommandProcessor = toCommandProcessor

		cu0 = mock_akita.NewMockPort(mockCtrl)
		cu1 = mock_akita.NewMockPort(mockCtrl)
		dispatcher.RegisterCU(cu0)
		dispatcher.RegisterCU(cu1)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("start kernel launching", func() {
		dispatcher.dispatchingReq = nil

		req := NewLaunchKernelReq(10, nil, dispatcher.ToCommandProcessor)
		req.SetRecvTime(10)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		dispatcher.Handle(req)
	})

	It("should map work-group", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq

		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		toCUs.EXPECT().Send(gomock.AssignableToTypeOf(&MapWGReq{}))

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should reschedule work-group mapping if sending failed", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq

		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups...)
		dispatcher.dispatchingCUID = -1

		toCUs.EXPECT().
			Send(gomock.AssignableToTypeOf(&MapWGReq{})).
			Return(&akita.SendError{})

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should do nothing if all work-groups are mapped", func() {
		dispatcher.dispatchingCUID = -1

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should do nothing if all cus are busy", func() {
		dispatcher.cuBusy[cu0] = true
		dispatcher.cuBusy[cu1] = true
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
			grid.WorkGroups[0])

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should mark CU busy if MapWGReq failed", func() {
		wg := grid.WorkGroups[0]
		dispatcher.dispatchingCUID = 0
		dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs, wg)

		req := NewMapWGReq(cu0, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)
		req.Ok = false

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		dispatcher.Handle(req)

		Expect(dispatcher.cuBusy[cu0]).To(BeTrue())
	})

	It("should map another work-group when finished mapping a work-group",
		func() {
			dispatcher.dispatchingCUID = 0
			dispatcher.dispatchingWGs = append(dispatcher.dispatchingWGs,
				grid.WorkGroups...)

			wg := grid.WorkGroups[0]
			req := NewMapWGReq(cu0, dispatcher.ToCUs, 10, wg)
			req.SetRecvTime(11)
			req.Ok = true

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

			dispatcher.Handle(req)
		})

	It("should continue dispatching when receiving WGFinishMesg", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.cuBusy[cu0] = true

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		dispatcher.Handle(req)

		Expect(dispatcher.cuBusy[cu0]).To(BeFalse())
	})

	It("should not continue dispatching when receiving WGFinishMesg and "+
		"the dispatcher is dispatching", func() {
		dispatcher.dispatchingGrid = grid
		dispatcher.state = DispatcherToMapWG

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)

		dispatcher.Handle(req)
	})

	It("should send the KernelLaunchingReq back to the command processor, "+
		"when receiving WGFinishMesg and there is no more work-groups", func() {
		kernelLaunchingReq := NewLaunchKernelReq(10,
			nil, dispatcher.ToCommandProcessor)
		dispatcher.dispatchingReq = kernelLaunchingReq
		dispatcher.dispatchingGrid = grid

		wg := grid.WorkGroups[0]
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)

		dispatcher.completedWGs = append(dispatcher.completedWGs,
			grid.WorkGroups[1:]...)

		toCommandProcessor.EXPECT().
			Send(gomock.AssignableToTypeOf(&LaunchKernelReq{}))

		dispatcher.Handle(req)

		Expect(dispatcher.dispatchingReq).To(BeNil())
	})
})
