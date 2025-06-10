package dispatching

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"go.uber.org/mock/gomock"
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

		dispatchingPort.EXPECT().AsRemote().AnyTimes()
		respondingPort.EXPECT().AsRemote().AnyTimes()

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

		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
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
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		Expect(func() { dispatcher.StartDispatching(req) }).To(Panic())
	})

	It("should dispatch work-groups", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: true,
			cu:    nilPort.AsRemote(),
		})
		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		dispatchingPort.EXPECT().Send(gomock.Any()).Return(nil)

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(1))
		Expect(dispatcher.inflightWGs).To(HaveLen(1))
		Expect(dispatcher.cycleLeft).NotTo(Equal(0))
	})

	It("should wait until cycle left becomes 0", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req
		dispatcher.cycleLeft = 3

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.cycleLeft).To(Equal(2))
	})

	It("should pause if no work-group can be executed", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: false,
			cu:    nilPort.AsRemote(),
		})

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should pause if send to CU failed", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: true,
			cu:    nilPort.AsRemote(),
		})
		dispatchingPort.EXPECT().
			Send(gomock.Any()).
			Return(sim.NewSendError())

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeTrue())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should do nothing if all work-groups dispatched", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(false).AnyTimes()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive work-group complete message", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
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
			PeekIncoming().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			RetrieveIncoming()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.inflightWGs).NotTo(HaveKey(mapWGReq.ID))
	})

	It(`should add kernel overhead after completing the last 
	Work-Group`, func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
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
			PeekIncoming().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			RetrieveIncoming()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.inflightWGs).NotTo(HaveKey(mapWGReq.ID))
		Expect(dispatcher.cycleLeft).
			To(Equal(dispatcher.constantKernelOverhead))
	})

	It(`should ignore response if the request is not sent by the 
	dispatcher`, func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		mapWGReq := protocol.MapWGReqBuilder{}.Build()
		// dispatcher.inflightWGs[mapWGReq.ID] = location

		wgCompletionMsg := &protocol.WGCompletionMsg{RspTo: []string{mapWGReq.ID}}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().
			PeekIncoming().
			Return(wgCompletionMsg)

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should send response when a kernel is completed", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		respondingPort.EXPECT().
			Send(gomock.Any()).
			Return(nil)

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.dispatching).To(BeNil())
	})

	It("should wait if response is failed to send", func() {
		nilPort := NewMockPort(ctrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(nilPort, respondingPort)
		dispatcher.dispatching = req

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		respondingPort.EXPECT().
			Send(gomock.Any()).
			Return(sim.NewSendError())

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.dispatching).To(BeIdenticalTo(req))
	})
})
