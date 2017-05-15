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
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		conn = core.NewMockConnection()
		execUnit = core.NewMockComponent("ExecUnit")
		decodeUnit = NewSimpleDecodeUnit("DecodeU", engine)
		decodeUnit.ExecUnit = execUnit
		decodeUnit.Freq = 1
		decodeUnit.Latency = 1
		core.PlugIn(decodeUnit, "ToExecUnit", conn)
	})

	It("should schedule decode completion event", func() {
		decodeUnit.available = true
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		issueInstReq.SetRecvTime(10)
		decodeUnit.Recv(issueInstReq)
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		Expect(decodeUnit.available).To(BeFalse())
		Expect(decodeUnit.nextPossibleTime).To(BeNumerically("~", 12, 1e-9))
	})

	It("should reject decode request if not available", func() {
		decodeUnit.available = false
		decodeUnit.nextPossibleTime = 14
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		issueInstReq.SetRecvTime(10)
		err := decodeUnit.Recv(issueInstReq)
		Expect(err).NotTo(BeNil())
		Expect(err.Recoverable).To(BeTrue())
		Expect(err.EarliestRetry).To(BeNumerically("~", 14, 1e-9))
	})

	It("should send IssueInstReq to the ExecUnit", func() {
		decodeUnit.available = false
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		reqToExpect := NewIssueInstReq(decodeUnit, execUnit, 11, nil, nil)
		conn.ExpectSend(reqToExpect, nil)

		decodeUnit.Handle(evt)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(decodeUnit.available).To(BeTrue())
	})

	It("should reschedule event if cannot send IssueInstReq", func() {
		decodeUnit.available = false
		issueInstReq := NewIssueInstReq(nil, decodeUnit, 10, nil, nil)
		evt := NewDecodeCompletionEvent(11, decodeUnit, issueInstReq)

		reqToExpect := NewIssueInstReq(decodeUnit, execUnit, 11, nil, nil)
		conn.ExpectSend(reqToExpect, core.NewError("err", true, 13))

		decodeUnit.Handle(evt)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(decodeUnit.available).To(BeFalse())
		Expect(decodeUnit.nextPossibleTime).To(BeNumerically("~", 14, 1e-9))
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
	})

})
