package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

var _ = Describe("Dispatcher", func() {
	var (
		mockCtrl    *gomock.Controller
		dispatcher  *Dispatcher
		engine      *MockEngine
		gridBuilder *MockGridBuilder

		toCommandProcessor *MockPort
		toCUs              *MockPort
		cu0                *MockPort
		cu1                *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)

		gridBuilder = NewMockGridBuilder(mockCtrl)

		toCommandProcessor = NewMockPort(mockCtrl)
		toCUs = NewMockPort(mockCtrl)

		dispatcher = NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1
		dispatcher.ToCUs = toCUs
		dispatcher.ToCommandProcessor = toCommandProcessor

		cu0 = NewMockPort(mockCtrl)
		cu1 = NewMockPort(mockCtrl)
		dispatcher.RegisterCU(cu0)
		dispatcher.RegisterCU(cu1)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("start kernel launching", func() {
		dispatcher.dispatchingReq = nil
		req := NewLaunchKernelReq(10, nil, dispatcher.ToCommandProcessor)
		req.HsaCo = &insts.HsaCo{}
		req.Packet = &kernels.HsaKernelDispatchPacket{}
		req.SetRecvTime(10)
		gridBuilder.EXPECT().NumWG().Return(5)
		gridBuilder.EXPECT().SetKernel(kernels.KernelLaunchInfo{
			CodeObject: req.HsaCo,
			Packet:     req.Packet,
			PacketAddr: req.PacketAddress,
		})
		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		dispatcher.Handle(req)

		Expect(dispatcher.totalWGs).To(Equal(5))
	})

	It("should map work-group", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq
		dispatcher.dispatchingCUID = -1

		gridBuilder.EXPECT().NextWG().Return(&kernels.WorkGroup{})
		toCUs.EXPECT().Send(gomock.AssignableToTypeOf(&MapWGReq{}))

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should reschedule work-group mapping if sending failed", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq

		gridBuilder.EXPECT().NextWG().Return(&kernels.WorkGroup{})
		toCUs.EXPECT().
			Send(gomock.AssignableToTypeOf(&MapWGReq{})).
			Return(&akita.SendError{})
		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should do nothing if all work-groups are mapped", func() {
		dispatcher.dispatchingCUID = -1

		gridBuilder.EXPECT().NextWG().Return(nil)

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should do nothing if all cus are busy", func() {
		dispatcher.cuBusy[cu0] = true
		dispatcher.cuBusy[cu1] = true

		gridBuilder.EXPECT().NextWG().Return(&kernels.WorkGroup{})

		evt := NewMapWGEvent(10, dispatcher)
		dispatcher.Handle(evt)
	})

	It("should mark CU busy if MapWGReq failed", func() {
		wg := &kernels.WorkGroup{}
		dispatcher.dispatchingCUID = 0
		dispatcher.currentWG = wg
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

			wg := &kernels.WorkGroup{}
			req := NewMapWGReq(cu0, dispatcher.ToCUs, 10, wg)
			req.SetRecvTime(11)
			req.Ok = true

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

			dispatcher.Handle(req)
		})

	It("should continue dispatching when receiving WGFinishMesg", func() {
		dispatcher.cuBusy[cu0] = true
		dispatcher.totalWGs = 10
		wg := &kernels.WorkGroup{}
		dispatchReq := NewMapWGReq(dispatcher.ToCUs, nil, 6, wg)
		dispatcher.dispatchedWGs[wg.UID] = dispatchReq
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)
		req.SetRecvTime(11)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(&MapWGEvent{}))

		dispatcher.Handle(req)

		Expect(dispatcher.cuBusy[cu0]).To(BeFalse())
	})

	It("should not continue dispatching when receiving WGFinishMesg and "+
		"the dispatcher is dispatching", func() {
		dispatcher.state = DispatcherToMapWG
		dispatcher.totalWGs = 10
		wg := &kernels.WorkGroup{}
		dispatchReq := NewMapWGReq(dispatcher.ToCUs, nil, 6, wg)
		dispatcher.dispatchedWGs[wg.UID] = dispatchReq
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)

		dispatcher.Handle(req)
	})

	It("should send the KernelLaunchingReq back to the command processor, "+
		"when receiving WGFinishMesg and there is no more work-groups", func() {
		kernelLaunchingReq := NewLaunchKernelReq(10,
			nil, dispatcher.ToCommandProcessor)
		dispatcher.dispatchingReq = kernelLaunchingReq
		dispatcher.totalWGs = 1

		wg := &kernels.WorkGroup{}
		dispatchReq := NewMapWGReq(dispatcher.ToCUs, nil, 6, wg)
		dispatcher.dispatchedWGs[wg.UID] = dispatchReq
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)

		toCommandProcessor.EXPECT().
			Send(gomock.AssignableToTypeOf(&LaunchKernelReq{}))

		dispatcher.Handle(req)

		Expect(dispatcher.dispatchingReq).To(BeNil())
		Expect(dispatcher.dispatchedWGs).To(HaveLen(0))
	})
})
