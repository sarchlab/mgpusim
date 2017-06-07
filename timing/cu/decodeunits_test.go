package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("SimpleDecodeUnit", func() {

	var (
		decodeUnit *SimpleDecodeUnit
		execUnit   *core.MockComponent
		engine     *core.MockEngine
		conn       *core.MockConnection
		wavefront  *Wavefront
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		conn = core.NewMockConnection()
		execUnit = core.NewMockComponent("ExecUnit")
		decodeUnit = NewSimpleDecodeUnit("DecodeU", engine)
		decodeUnit.ExecUnit = execUnit
		decodeUnit.Freq = 1
		decodeUnit.Latency = 1
		wavefront = new(Wavefront)
		core.PlugIn(decodeUnit, "ToExecUnit", conn)
	})

	It("should schedule decode completion event", func() {
		decodeUnit.toDecode = nil
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wavefront)
		issueInstReq.SetRecvTime(10.5)
		decodeUnit.Recv(issueInstReq)
		Expect(decodeUnit.toDecode).To(BeIdenticalTo(wavefront))
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 11, 1e-12))
	})

	It("should reject decode request if not available", func() {
		wf := new(Wavefront)
		decodeUnit.toDecode = wf
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wf)
		issueInstReq.SetRecvTime(10)
		err := decodeUnit.Recv(issueInstReq)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeTrue())
		Expect(err.EarliestRetry).To(BeNumerically("~", 11, 1e-9))
	})

	It("should decode", func() {
		wf := new(Wavefront)
		decodeUnit.toDecode = wf
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wf)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		decodeUnit.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 11.5, 1e-12))
		Expect(decodeUnit.decoded).To(BeIdenticalTo(wf))
		Expect(decodeUnit.toDecode).To(BeNil())
	})

	It("should reschedule decoding if output is not cleared", func() {
		wf := new(Wavefront)
		decodeUnit.decoded = wf
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wf)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		decodeUnit.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 12, 1e-12))
	})

	It("should handle deferredSend", func() {
		issueInstReq := NewIssueInstReq(decodeUnit, nil, 11.5, nil, nil)
		deferredSend := core.NewDeferredSend(issueInstReq)
		conn.ExpectSend(issueInstReq, nil)

		decodeUnit.Handle(deferredSend)

		Expect(conn.AllExpectedSent()).To(BeTrue())
	})

	It("should reschedule deferredSend if send failed", func() {
		issueInstReq := NewIssueInstReq(decodeUnit, nil, 11.5, nil, nil)
		deferredSend := core.NewDeferredSend(issueInstReq)

		conn.ExpectSend(issueInstReq, core.NewError("err", true, 13))

		decodeUnit.Handle(deferredSend)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 13.5, 1e-12))
	})
})

var _ = Describe("VectorDecodeUnit", func() {

	var (
		decodeUnit *VectorDecodeUnit
		simdUnits  []*core.MockComponent
		engine     *core.MockEngine
		conn       *core.MockConnection
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		conn = core.NewMockConnection()
		decodeUnit = NewVectorDecodeUnit("DecodeU", engine)
		for i := 0; i < 4; i++ {
			simdUnit := core.NewMockComponent("simd")
			simdUnits = append(simdUnits, simdUnit)
			decodeUnit.SIMDUnits = append(decodeUnit.SIMDUnits, simdUnit)
		}
		decodeUnit.Freq = 1
		decodeUnit.Latency = 1
		core.PlugIn(decodeUnit, "ToExecUnit", conn)
	})

	It("should schedule decode completion event", func() {
		decodeUnit.toDecode = nil
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		issueInstReq.SetRecvTime(10.5)
		decodeUnit.Recv(issueInstReq)
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 11, 1e-12))
	})

	It("should reject decode request if not available", func() {
		decodeUnit.toDecode = new(Wavefront)
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		issueInstReq.SetRecvTime(10)
		err := decodeUnit.Recv(issueInstReq)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeTrue())
		Expect(err.EarliestRetry).To(BeNumerically("~", 11, 1e-9))
	})

	It("should send IssueInstReq to the ExecUnit", func() {
		wf := new(Wavefront)
		wf.SIMDID = 0
		decodeUnit.toDecode = wf

		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wf)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		decodeUnit.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 11.5, 1e-12))
		Expect(decodeUnit.decoded).To(BeIdenticalTo(wf))
		Expect(decodeUnit.toDecode).To(BeNil())
	})

	It("should reschedule event if cannot send IssueInstReq", func() {
		wf := new(Wavefront)
		wf.SIMDID = 0
		decodeUnit.decoded = wf

		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, wf)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		decodeUnit.Handle(evt)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 12, 1e-12))
	})

	It("should handle deferred send", func() {
		wf := new(Wavefront)
		wf.SIMDID = 0
		decodeUnit.decoded = wf

		issueInstReq := NewIssueInstReq(nil, decodeUnit, 11.5, nil, wf)
		deferredSend := core.NewDeferredSend(issueInstReq)

		conn.ExpectSend(issueInstReq, nil)

		decodeUnit.Handle(deferredSend)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(decodeUnit.decoded).To(BeNil())
	})

	It("should reschedule deferred send, if send is not successful", func() {
		wf := new(Wavefront)
		wf.SIMDID = 0
		decodeUnit.decoded = wf

		issueInstReq := NewIssueInstReq(nil, decodeUnit, 11.5, nil, wf)
		deferredSend := core.NewDeferredSend(issueInstReq)
		conn.ExpectSend(issueInstReq, core.NewError("err", true, 13))

		decodeUnit.Handle(deferredSend)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 13.5, 1e-12))
		Expect(conn.AllExpectedSent()).To(BeTrue())
	})

})
