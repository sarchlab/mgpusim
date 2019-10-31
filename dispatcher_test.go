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
		req.RecvTime = 10
		gridBuilder.EXPECT().NumWG().Return(5)
		gridBuilder.EXPECT().SetKernel(kernels.KernelLaunchInfo{
			CodeObject: req.HsaCo,
			Packet:     req.Packet,
			PacketAddr: req.PacketAddress,
		})

		toCommandProcessor.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := dispatcher.processLaunchKernelReq(10, req)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.totalWGs).To(Equal(5))
		Expect(dispatcher.state).To(Equal(dispatcherToMapWG))
	})

	It("should map work-group", func() {
		dispatchingReq := NewLaunchKernelReq(10, nil, nil)
		dispatcher.dispatchingReq = dispatchingReq
		dispatcher.dispatchingCUID = -1
		dispatcher.state = dispatcherToMapWG

		gridBuilder.EXPECT().NextWG().Return(&kernels.WorkGroup{})
		toCUs.EXPECT().Send(gomock.AssignableToTypeOf(&MapWGReq{}))

		madeProgress := dispatcher.mapWG(10)

		Expect(madeProgress).To(BeTrue())
	})

	It("should mark CU busy if MapWGReq failed", func() {
		wg := &kernels.WorkGroup{}
		dispatcher.dispatchingCUID = 0
		dispatcher.currentWG = wg
		dispatcher.state = dispatcherWaitMapWGACK
		req := NewMapWGReq(cu0, dispatcher.ToCUs, 10, wg)
		req.RecvTime = 11
		req.Ok = false

		toCUs.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := dispatcher.processMapWGRsp(10, req)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.cuBusy[cu0]).To(BeTrue())
		Expect(dispatcher.state).To(Equal(dispatcherToMapWG))
	})

	It("should map another work-group when finished mapping a work-group",
		func() {
			dispatcher.dispatchingCUID = 0

			wg := &kernels.WorkGroup{}
			req := NewMapWGReq(cu0, dispatcher.ToCUs, 10, wg)
			req.RecvTime = 11
			req.Ok = true

			toCUs.EXPECT().Retrieve(akita.VTimeInSec(10))

			madeProgress := dispatcher.processMapWGRsp(10, req)

			Expect(madeProgress).To(Equal(true))
			Expect(dispatcher.currentWG).To(BeNil())
			Expect(dispatcher.state).To(Equal(dispatcherToMapWG))
		})

	It("should continue dispatching when receiving WGFinishMesg", func() {
		dispatcher.cuBusy[cu0] = true
		dispatcher.totalWGs = 10
		wg := &kernels.WorkGroup{}
		dispatchReq := NewMapWGReq(dispatcher.ToCUs, nil, 6, wg)
		dispatcher.dispatchedWGs[wg.UID] = dispatchReq
		req := NewWGFinishMesg(cu0, dispatcher.ToCUs, 10, wg)
		req.RecvTime = 11

		toCUs.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := dispatcher.processWGFinishMesg(10, req)

		Expect(dispatcher.cuBusy[cu0]).To(BeFalse())
		Expect(madeProgress).To(BeTrue())
	})
})
