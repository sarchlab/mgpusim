package dispatching

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"go.uber.org/mock/gomock"
)

// fakePortSource provides the mocked ports of the dispatcher by name.
type fakePortSource struct {
	ports map[string]messaging.Port
}

func (s *fakePortSource) GetPortByName(name string) messaging.Port {
	return s.ports[name]
}

var _ = Describe("Dispatcher", func() {
	var (
		ctrl *gomock.Controller

		cp              *MockNamedHookable
		alg             *MockAlgorithm
		dispatchingPort *MockPort
		respondingPort  *MockPort

		dispatcher *DispatcherImpl
	)

	makeLaunchReq := func() protocol.LaunchKernelReq {
		return protocol.LaunchKernelReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: "Driver",
				Dst: "CP.ToDriver",
			},
		}
	}

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		cp = NewMockNamedHookable(ctrl)
		cp.EXPECT().Name().Return("CP").AnyTimes()
		cp.EXPECT().NumHooks().Return(0).AnyTimes()
		cp.EXPECT().InvokeHook(gomock.Any()).AnyTimes()
		alg = NewMockAlgorithm(ctrl)
		dispatchingPort = NewMockPort(ctrl)
		respondingPort = NewMockPort(ctrl)

		dispatchingPort.EXPECT().AsRemote().
			Return(messaging.RemotePort("CP.ToCUs")).AnyTimes()
		respondingPort.EXPECT().AsRemote().
			Return(messaging.RemotePort("CP.ToDriver")).AnyTimes()

		portSource := &fakePortSource{ports: map[string]messaging.Port{
			"ToCUs":    dispatchingPort,
			"ToDriver": respondingPort,
		}}

		dispatcher = MakeBuilder().
			WithCP(cp).
			WithPortSource(portSource).
			WithDispatchingPortName("ToCUs").
			WithRespondingPortName("ToDriver").
			Build("dispatcher").(*DispatcherImpl)

		dispatcher.alg = alg
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should start dispatching a new kernel", func() {
		hsaco := &insts.KernelCodeObject{
			KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
		}
		packet := &kernels.HsaKernelDispatchPacket{}
		packetAddr := uint64(0x40)

		req := makeLaunchReq()
		req.CodeObject = hsaco
		req.Packet = packet
		req.PacketAddress = packetAddr

		alg.EXPECT().StartNewKernel(kernels.KernelLaunchInfo{
			CodeObject: hsaco,
			Packet:     packet,
			PacketAddr: packetAddr,
		})

		dispatcher.StartDispatching(req)

		Expect(dispatcher.isDispatching).To(BeTrue())
		Expect(dispatcher.dispatching.ID).To(Equal(req.ID))
	})

	It("should panic if the dispatcher is dispatching another kernel", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		Expect(func() { dispatcher.StartDispatching(req) }).To(Panic())
	})

	It("should dispatch work-groups", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		alg.EXPECT().HasNext().Return(true).AnyTimes()
		firstCall := alg.EXPECT().Next().Return(dispatchLocation{
			valid:     true,
			cu:        "CU0",
			locations: make([]protocol.WfDispatchLocation, 1),
		})
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: false,
		}).After(firstCall).AnyTimes()
		dispatchingPort.EXPECT().PeekIncoming().Return(nil).AnyTimes()
		dispatchingPort.EXPECT().CanSend().Return(true)
		dispatchingPort.EXPECT().Send(gomock.Any())

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(1))
		Expect(dispatcher.inflightWGs).To(HaveLen(1))
	})

	It("should wait until cycle left becomes 0", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true
		dispatcher.cycleLeft = 3

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.cycleLeft).To(Equal(2))
	})

	It("should pause if no work-group can be executed", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: false,
			cu:    "CU0",
		})

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeFalse())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should pause if send to CU failed", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(true).AnyTimes()
		alg.EXPECT().Next().Return(dispatchLocation{
			valid: true,
			cu:    "CU0",
		})
		dispatchingPort.EXPECT().CanSend().Return(false)

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.currWG.valid).To(BeTrue())
		Expect(dispatcher.numDispatchedWGs).To(Equal(0))
	})

	It("should do nothing if all work-groups dispatched", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		alg.EXPECT().HasNext().Return(false).AnyTimes()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive work-group complete message", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		mapWGReq := protocol.MapWGReq{
			MsgMeta: messaging.MsgMeta{
				ID: timing.GetIDGenerator().Generate(),
			},
		}
		location := dispatchLocation{}
		dispatcher.inflightWGs[mapWGReq.ID] = location
		dispatcher.originalReqs[mapWGReq.ID] = mapWGReq

		wgCompletionMsg := protocol.WGCompletionMsg{
			RspToIDs: []uint64{mapWGReq.ID},
		}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		alg.EXPECT().NumWG().Return(64)
		alg.EXPECT().FreeResources(location)

		firstPeek := dispatchingPort.EXPECT().
			PeekIncoming().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			PeekIncoming().
			Return(nil).
			After(firstPeek).
			AnyTimes()
		dispatchingPort.EXPECT().
			RetrieveIncoming()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.inflightWGs).NotTo(HaveKey(mapWGReq.ID))
	})

	It(`should add kernel overhead after completing the last
	Work-Group`, func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		mapWGReq := protocol.MapWGReq{
			MsgMeta: messaging.MsgMeta{
				ID: timing.GetIDGenerator().Generate(),
			},
		}
		location := dispatchLocation{}
		dispatcher.inflightWGs[mapWGReq.ID] = location
		dispatcher.originalReqs[mapWGReq.ID] = mapWGReq

		wgCompletionMsg := protocol.WGCompletionMsg{
			RspToIDs: []uint64{mapWGReq.ID},
		}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 63

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		alg.EXPECT().NumWG().Return(64)
		alg.EXPECT().FreeResources(location)

		firstPeek := dispatchingPort.EXPECT().
			PeekIncoming().
			Return(wgCompletionMsg)
		dispatchingPort.EXPECT().
			PeekIncoming().
			Return(nil).
			After(firstPeek).
			AnyTimes()
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
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		mapWGReq := protocol.MapWGReq{
			MsgMeta: messaging.MsgMeta{
				ID: timing.GetIDGenerator().Generate(),
			},
		}

		wgCompletionMsg := protocol.WGCompletionMsg{
			RspToIDs: []uint64{mapWGReq.ID},
		}

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 48

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().
			PeekIncoming().
			Return(wgCompletionMsg).
			AnyTimes()

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should send response when a kernel is completed", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		respondingPort.EXPECT().CanSend().Return(true)
		respondingPort.EXPECT().Send(gomock.Any()).
			Do(func(rsp messaging.Msg) {
				launchRsp := rsp.(protocol.LaunchKernelRsp)
				Expect(launchRsp.RspTo).To(Equal(req.ID))
				Expect(launchRsp.Dst).To(Equal(req.Src))
			})

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(dispatcher.isDispatching).To(BeFalse())
	})

	It("should wait if response is failed to send", func() {
		req := makeLaunchReq()
		dispatcher.dispatching = req
		dispatcher.isDispatching = true

		dispatcher.numDispatchedWGs = 64
		dispatcher.numCompletedWGs = 64

		alg.EXPECT().HasNext().Return(false).AnyTimes()
		dispatchingPort.EXPECT().PeekIncoming().Return(nil)
		respondingPort.EXPECT().CanSend().Return(false)

		madeProgress := dispatcher.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(dispatcher.isDispatching).To(BeTrue())
		Expect(dispatcher.dispatching.ID).To(Equal(req.ID))
	})
})
