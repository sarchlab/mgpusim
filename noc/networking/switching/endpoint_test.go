package switching

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
)

var _ = Describe("End Point", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		devicePort        *MockPort
		networkPort       *MockPort
		defaultSwitchPort *MockPort
		endPoint          *EndPoint
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		devicePort = NewMockPort(mockCtrl)
		networkPort = NewMockPort(mockCtrl)
		defaultSwitchPort = NewMockPort(mockCtrl)

		devicePort.EXPECT().SetConnection(gomock.Any())

		endPoint = MakeEndPointBuilder().
			WithEngine(engine).
			WithFreq(1).
			WithFlitByteSize(32).
			WithDevicePorts([]sim.Port{devicePort}).
			Build("EndPoint")
		endPoint.NetworkPort = networkPort
		endPoint.DefaultSwitchDst = defaultSwitchPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send flits", func() {
		msg := &sampleMsg{}
		msg.TrafficBytes = 33

		networkPort.EXPECT().Peek().Return(nil).AnyTimes()

		engine.EXPECT().Schedule(gomock.Any())
		endPoint.Send(msg)

		madeProgress := endPoint.Tick(10)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.SendTime).To(Equal(sim.VTimeInSec(11)))
			Expect(flit.Src).To(Equal(networkPort))
			Expect(flit.Dst).To(Equal(defaultSwitchPort))
			Expect(flit.SeqID).To(Equal(0))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})
		devicePort.EXPECT().NotifyAvailable(gomock.Any())

		madeProgress = endPoint.Tick(11)
		Expect(madeProgress).To(BeTrue())

		networkPort.EXPECT().Send(gomock.Any()).Do(func(flit *messaging.Flit) {
			Expect(flit.SendTime).To(Equal(sim.VTimeInSec(12)))
			Expect(flit.Src).To(Equal(networkPort))
			Expect(flit.Dst).To(Equal(defaultSwitchPort))
			Expect(flit.SeqID).To(Equal(1))
			Expect(flit.NumFlitInMsg).To(Equal(2))
			Expect(flit.Msg).To(BeIdenticalTo(msg))
		})

		madeProgress = endPoint.Tick(12)

		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(13)

		Expect(madeProgress).To(BeFalse())
	})

	It("should receive message", func() {
		msg := &sampleMsg{}
		msg.Dst = devicePort

		flit0 := messaging.FlitBuilder{}.
			WithSeqID(0).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()
		flit1 := messaging.FlitBuilder{}.
			WithSeqID(0).
			WithNumFlitInMsg(2).
			WithMsg(msg).
			Build()

		networkPort.EXPECT().Peek().Return(flit0)
		networkPort.EXPECT().Peek().Return(flit1)
		networkPort.EXPECT().Peek().Return(nil).Times(3)
		networkPort.EXPECT().Retrieve(gomock.Any()).Times(2)
		devicePort.EXPECT().Recv(msg)

		madeProgress := endPoint.Tick(10)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(11)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(12)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(13)
		Expect(madeProgress).To(BeTrue())

		madeProgress = endPoint.Tick(14)
		Expect(madeProgress).To(BeFalse())
	})
})
