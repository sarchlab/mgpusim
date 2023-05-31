package switching

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
)

func createMockPortComplex(ctrl *gomock.Controller) portComplex {
	local := NewMockPort(ctrl)
	remote := NewMockPort(ctrl)
	routeBuf := NewMockBuffer(ctrl)
	forwardBuf := NewMockBuffer(ctrl)
	sendOutBuf := NewMockBuffer(ctrl)
	pipeline := NewMockPipeline(ctrl)

	pc := portComplex{
		localPort:        local,
		remotePort:       remote,
		pipeline:         pipeline,
		routeBuffer:      routeBuf,
		forwardBuffer:    forwardBuf,
		sendOutBuffer:    sendOutBuf,
		numInputChannel:  1,
		numOutputChannel: 1,
	}

	return pc
}

var _ = Describe("Switch", func() {
	var (
		mockCtrl                   *gomock.Controller
		engine                     *MockEngine
		portComplex1, portComplex2 portComplex
		dstPort                    *MockPort
		routingTable               *MockTable
		arbiter                    *MockArbiter
		sw                         *Switch
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		portComplex1 = createMockPortComplex(mockCtrl)
		portComplex2 = createMockPortComplex(mockCtrl)
		dstPort = NewMockPort(mockCtrl)
		dstPort.EXPECT().Name().AnyTimes()
		routingTable = NewMockTable(mockCtrl)
		arbiter = NewMockArbiter(mockCtrl)
		arbiter.EXPECT().AddBuffer(gomock.Any()).AnyTimes()
		sw = SwitchBuilder{}.
			WithEngine(engine).
			WithFreq(1).
			WithRoutingTable(routingTable).
			WithArbiter(arbiter).
			Build("Switch")
		sw.addPort(portComplex1)
		sw.addPort(portComplex2)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should start processing", func() {
		port1 := portComplex1.localPort.(*MockPort)
		port2 := portComplex2.localPort.(*MockPort)
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithDst(port1).
			WithMsg(msg).
			Build()

		port1.EXPECT().Peek().Return(flit)
		port1.EXPECT().Retrieve(gomock.Any())
		port2.EXPECT().Peek().Return(nil)
		port1Pipeline.EXPECT().CanAccept().Return(true)
		port1Pipeline.
			EXPECT().
			Accept(sim.VTimeInSec(10), gomock.Any()).
			Do(func(now sim.VTimeInSec, i flitPipelineItem) {
				Expect(i.flit).To(Equal(flit))
			})

		madeProgress := sw.startProcessing(10)

		Expect(madeProgress).To(BeTrue())
	})

	It("should not start processing if pipeline is busy", func() {
		port1 := portComplex1.localPort.(*MockPort)
		port2 := portComplex2.localPort.(*MockPort)
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithDst(port1).
			WithMsg(msg).
			Build()

		port1.EXPECT().Peek().Return(flit)
		port2.EXPECT().Peek().Return(nil)
		port1Pipeline.EXPECT().CanAccept().Return(false)

		madeProgress := sw.startProcessing(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should tick the pipelines", func() {
		port1Pipeline := portComplex1.pipeline.(*MockPipeline)
		port2Pipeline := portComplex2.pipeline.(*MockPipeline)

		port1Pipeline.EXPECT().Tick(sim.VTimeInSec(10)).Return(false)
		port2Pipeline.EXPECT().Tick(sim.VTimeInSec(10)).Return(true)

		madeProgress := sw.movePipeline(10)

		Expect(madeProgress).To(BeTrue())
	})

	It("should route", func() {
		routeBuffer1 := portComplex1.routeBuffer.(*MockBuffer)
		routeBuffer2 := portComplex2.routeBuffer.(*MockBuffer)
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		pipelineItem := flitPipelineItem{taskID: "flit", flit: flit}
		routeBuffer1.EXPECT().Peek().Return(pipelineItem)
		routeBuffer1.EXPECT().Pop()
		routeBuffer2.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().CanPush().Return(true)
		forwardBuffer1.EXPECT().Push(flit)
		routingTable.EXPECT().FindPort(dstPort).Return(portComplex2.localPort)

		madeProgress := sw.route(10)

		Expect(madeProgress).To(BeTrue())
		Expect(flit.OutputBuf).To(BeIdenticalTo(portComplex2.sendOutBuffer))
	})

	It("should not route if forward buffer is full", func() {
		routeBuffer1 := portComplex1.routeBuffer.(*MockBuffer)
		routeBuffer2 := portComplex2.routeBuffer.(*MockBuffer)
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		pipelineItem := flitPipelineItem{taskID: "flit", flit: flit}
		routeBuffer1.EXPECT().Peek().Return(pipelineItem)
		routeBuffer2.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().CanPush().Return(false)

		madeProgress := sw.route(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should forward", func() {
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)
		forwardBuffer2 := portComplex2.forwardBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()
		flit.OutputBuf = sendOutBuffer2

		arbiter.EXPECT().
			Arbitrate(sim.VTimeInSec(10)).
			Return([]sim.Buffer{forwardBuffer1, forwardBuffer2})
		forwardBuffer1.EXPECT().Peek().Return(flit)
		forwardBuffer1.EXPECT().Peek().Return(nil)
		forwardBuffer1.EXPECT().Pop()
		forwardBuffer2.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().CanPush().Return(true)
		sendOutBuffer2.EXPECT().Push(flit)

		madeProgress := sw.forward(10)

		Expect(madeProgress).To(BeTrue())
	})

	It("should not forward if the output buffer is busy", func() {
		forwardBuffer1 := portComplex1.forwardBuffer.(*MockBuffer)
		forwardBuffer2 := portComplex2.forwardBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()
		flit.OutputBuf = sendOutBuffer2

		arbiter.EXPECT().
			Arbitrate(sim.VTimeInSec(10)).
			Return([]sim.Buffer{forwardBuffer1, forwardBuffer2})
		forwardBuffer1.EXPECT().Peek().Return(flit)
		forwardBuffer2.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().CanPush().Return(false)

		madeProgress := sw.forward(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should send flits out", func() {
		sendOutBuffer1 := portComplex1.sendOutBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)
		localPort2 := portComplex2.localPort.(*MockPort)
		remotePort2 := portComplex2.remotePort.(*MockPort)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		sendOutBuffer1.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().Peek().Return(flit)
		sendOutBuffer2.EXPECT().Pop()
		localPort2.EXPECT().Send(flit).Return(nil)

		madeProgress := sw.sendOut(10)

		Expect(madeProgress).To(BeTrue())
		Expect(flit.Dst).To(BeIdenticalTo(remotePort2))
		Expect(flit.Src).To(BeIdenticalTo(portComplex2.localPort))
		Expect(flit.SendTime).To(Equal(sim.VTimeInSec(10)))
	})

	It("should wait ifport is busy flits out", func() {
		sendOutBuffer1 := portComplex1.sendOutBuffer.(*MockBuffer)
		sendOutBuffer2 := portComplex2.sendOutBuffer.(*MockBuffer)
		localPort2 := portComplex2.localPort.(*MockPort)
		remotePort2 := portComplex2.remotePort.(*MockPort)

		msg := &sampleMsg{}
		msg.Src = dstPort
		msg.Dst = dstPort
		flit := messaging.FlitBuilder{}.
			WithMsg(msg).
			Build()

		sendOutBuffer1.EXPECT().Peek().Return(nil)
		sendOutBuffer2.EXPECT().Peek().Return(flit)
		localPort2.EXPECT().Send(flit).Return(&sim.SendError{})

		madeProgress := sw.sendOut(10)

		Expect(madeProgress).To(BeFalse())
		Expect(flit.Dst).To(BeIdenticalTo(remotePort2))
		Expect(flit.Src).To(BeIdenticalTo(portComplex2.localPort))
		Expect(flit.SendTime).To(Equal(sim.VTimeInSec(10)))
	})
})
