package messaging

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
)

type testMsg struct {
	sim.MsgMeta
}

func (m *testMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

var _ = Describe("Channel", func() {
	var (
		c *Channel

		mockCtrl                 *gomock.Controller
		engine                   *MockEngine
		src, dst                 *MockPort
		srcPipeline, dstPipeline *MockPipeline
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		src = NewMockPort(mockCtrl)
		dst = NewMockPort(mockCtrl)
		srcPipeline = NewMockPipeline(mockCtrl)
		dstPipeline = NewMockPipeline(mockCtrl)

		src.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		dst.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		src.EXPECT().Name().AnyTimes().Return("Src")
		dst.EXPECT().Name().AnyTimes().Return("Dst")

		c = MakeChannelBuilder().
			WithFreq(1).
			WithEngine(engine).
			WithPipelineParameters(1, 10, 1).
			Build("Channel")

		c.PlugIn(src, 1)
		c.PlugIn(dst, 1)

		c.left.pipeline = srcPipeline
		c.right.pipeline = dstPipeline
	})

	It("should send", func() {
		msg := &testMsg{}
		msg.Src = src
		msg.Dst = dst

		engine.EXPECT().Schedule(gomock.Any())

		c.Send(msg)
	})

	It("should tick", func() {
		srcPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))
		dstPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))

		madeProgress := c.Tick(1)

		Expect(madeProgress).To(BeFalse())
	})

	It("should deliver", func() {
		msg := &testMsg{}
		msg.Src = src
		msg.Dst = dst

		srcPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))
		dstPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))
		c.left.postPipelineBuf.Push(msgPipeTask{msg})

		dst.EXPECT().Recv(gomock.Any()).Do(func(msg *testMsg) {
			Expect(msg.RecvTime).To(Equal(sim.VTimeInSec(1.0)))
		})

		madeProgress := c.Tick(1)

		Expect(madeProgress).To(BeTrue())
		Expect(c.left.postPipelineBuf.Size()).To(Equal(0))
	})

	It("should move message to pipeline", func() {
		msg := &testMsg{}
		msg.Src = src
		msg.Dst = dst

		c.left.srcSideBuf.Push(msg)
		srcPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))
		dstPipeline.EXPECT().Tick(sim.VTimeInSec(1.0))
		srcPipeline.EXPECT().CanAccept().Return(true)
		srcPipeline.EXPECT().
			Accept(sim.VTimeInSec(1.0), msgPipeTask{msg: msg})

		madeProgress := c.Tick(1)

		Expect(madeProgress).To(BeTrue())
	})
})

var _ = Describe("Channel Integration", func() {
	var (
		c *Channel

		mockCtrl *gomock.Controller
		src, dst *MockPort
		engine   *sim.SerialEngine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		src = NewMockPort(mockCtrl)
		dst = NewMockPort(mockCtrl)
		src.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		dst.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		src.EXPECT().Name().AnyTimes().Return("Src")
		dst.EXPECT().Name().AnyTimes().Return("Dst")

		engine = sim.NewSerialEngine()

		c = MakeChannelBuilder().
			WithEngine(engine).
			WithFreq(1).
			WithPipelineParameters(100, 1, 1).
			Build("Channel")

		c.PlugIn(src, 1)
		c.PlugIn(dst, 1)
	})

	It("should deliver messages", func() {
		msg := &testMsg{}
		msg.Src = src
		msg.Dst = dst
		msg.SendTime = 0

		dst.EXPECT().Recv(msg).Do(func(msg *testMsg) {
			Expect(msg.RecvTime).To(Equal(sim.VTimeInSec(101)))
		})

		c.Send(msg)

		engine.Run()
	})
})
