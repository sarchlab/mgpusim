package dispatching

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/insts"
	"github.com/sarchlab/mgpusim/v4/kernels"
	"github.com/sarchlab/mgpusim/v4/protocol"
)

var _ = Describe("Dispatcher", func() {
	var (
		ctrl *gomock.Controller

		cp              *MockNamedHookable
		alg             *MockAlgorithm
		dispatchingPort *MockPort
		respondingPort  *MockPort

		dispatcher *DispatcherImpl
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		cp = NewMockNamedHookable(ctrl)
		cp.EXPECT().Name().Return("CP").AnyTimes()
		cp.EXPECT().NumHooks().Return(0).AnyTimes()
		cp.EXPECT().InvokeHook(gomock.Any()).AnyTimes()
		alg = NewMockAlgorithm(ctrl)
		dispatchingPort = NewMockPort(ctrl)
		respondingPort = NewMockPort(ctrl)

		dispatcher = MakeBuilder().
			WithCP(cp).
			WithDispatchingPort(dispatchingPort).
			WithRespondingPort(respondingPort).
			Build("dispatcher").(*DispatcherImpl)

		dispatcher.alg = alg

	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should start dispatching a new kernel", func() {
		hsaco := insts.NewHsaCo()
		packet := &kernels.HsaKernelDispatchPacket{}
		packetAddr := uint64(0x40)

		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		req.HsaCo = hsaco
		req.Packet = packet
		req.PacketAddress = packetAddr

		alg.EXPECT().StartNewKernel(kernels.KernelLaunchInfo{
			CodeObject: hsaco,
			Packet:     packet,
			PacketAddr: packetAddr,
		})

		dispatcher.StartDispatching(req)

		Expect(dispatcher.dispatching).To(BeIdenticalTo(req))
	})

	It("should panic if the dispatcher is dispatching another kernel", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		Expect(func() { dispatcher.StartDispatching(req) }).To(Panic())
	})

	It("should dispatch work-groups", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: true,
		})
		dispatchingPort.EXPECT().Peek().Return(nil)
		dispatchingPort.EXPECT().Send(gomock.Any()).Return(nil)

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(1))
		Expect(dispatcher.inflightWGs).To(HaveLen(1))
		Expect(dispatcher.cycleLeft).NotTo(Equal(0))
	})

	It("should wait until cycle left becomes 0", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req
		dispatcher.cycleLeft = 3

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.cycleLeft).To(Equal(2))
	})

	It("should pause if no work-group can be executed", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		dispatchingPort.EXPECT().Peek().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: false,
		})

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should pause if send to CU failed", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		dispatchingPort.EXPECT().Peek().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: true,
		})
		dispatchingPort.EXPECT().
			Send(gomock.Any()).
			Return(sim.NewSendError())

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeTrue())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should do nothing if all work-groups dispatched", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		dispatchingPort.EXPECT().Peek().Return(nil)
		alg.EXPECT().HasNext().Return(false).AnyTimes()

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive work-group complete message", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		mapWGReq := protocol.MapWGReqBuilder{}.Build()
		location := dispatchLocation{}
		dispatcher.inflightWGs[mapWGReq.ID] = location
		dispatcher.originalReqs[mapWGReq.ID] = mapWGReq

		wgCompletionMsg := &protocol.WGCompletionMsg{RspTo: []string{mapWGReq.ID}}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		alg.EXPECT().NumWG().Return(64)
		alg.EXPECT().FreeResources(location)
		dispatchingPort.EXPECT().
			Peek().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			Retrieve(sim.VTimeInSec(10))

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.inflightWGs).NotTo(HaveKey(mapWGReq.ID))
	})

	It(`should add kernel overhead after completing the last 
	Work-Group`, func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		mapWGReq := protocol.MapWGReqBuilder{}.Build()
		location := dispatchLocation{}
		dispatcher.inflightWGs[mapWGReq.ID] = location
		dispatcher.originalReqs[mapWGReq.ID] = mapWGReq

		wgCompletionMsg := &protocol.WGCompletionMsg{RspTo: []string{mapWGReq.ID}}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 63

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		alg.EXPECT().NumWG().Return(64)
		alg.EXPECT().FreeResources(location)
		dispatchingPort.EXPECT().
			Peek().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			Retrieve(sim.VTimeInSec(10))

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.inflightWGs).NotTo(HaveKey(mapWGReq.ID))
		Expect(dispatcher.cycleLeft).
			To(Equal(dispatcher.constantKernelOverhead))
	})

	It(`should ignore response if the request is not sent by the 
	dispatcher`, func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		mapWGReq := protocol.MapWGReqBuilder{}.Build()
		// dispatcher.inflightWGs[mapWGReq.ID] = location

		wgCompletionMsg := &protocol.WGCompletionMsg{RspTo: []string{mapWGReq.ID}}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().
			Peek().
			Return(wgCompletionMsg)

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should send response when a kernel is completed", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().Peek().Return(nil)
		respondingPort.EXPECT().
			Send(gomock.Any()).
			Return(nil)

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.dispatching).To(BeNil())
	})

	It("should wait if response is failed to send", func() {
		req := protocol.NewLaunchKernelReq(10, nil, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().Peek().Return(nil)
		respondingPort.EXPECT().
			Send(gomock.Any()).
			Return(sim.NewSendError())

		madeProgress := dispatcher.Tick(10)

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.dispatching).To(BeIdenticalTo(req))
	})
})
