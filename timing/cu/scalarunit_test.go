package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("Scalar Unit", func() {
	var (
		engine    *core.MockEngine
		scheduler *core.MockComponent
		conn      *core.MockConnection
		unit      *ScalarUnit
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		scheduler = core.NewMockComponent("scheduler")
		conn = core.NewMockConnection()
		unit = NewScalarUnit("branch_unit", engine, scheduler)
		core.PlugIn(unit, "ToScheduler", conn)
		unit.Freq = 1
	})

	It("should not accept instruction if buffer is occupied", func() {
		wf := new(Wavefront)
		unit.reading = wf

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).NotTo(BeNil())
	})

	It("should accept instruction if buffer is not occupied", func() {
		wf := new(Wavefront)

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).To(BeNil())
		Expect(unit.reading).To(BeIdenticalTo(wf))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should do read", func() {
		wf := new(Wavefront)
		unit.reading = wf
		unit.running = true

		evt := core.NewTickEvent(10, unit)
		unit.Handle(evt)

		Expect(unit.reading).To(BeNil())
		Expect(unit.executing).To(BeIdenticalTo(wf))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should do exec", func() {
		wf := new(Wavefront)
		unit.executing = wf
		unit.running = true

		evt := core.NewTickEvent(10, unit)
		unit.Handle(evt)

		Expect(unit.executing).To(BeNil())
		Expect(unit.writing).To(BeIdenticalTo(wf))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should do write", func() {
		wf := new(Wavefront)
		unit.writing = wf
		unit.running = true

		req := NewInstCompletionReq(unit, scheduler, 10, wf)
		conn.ExpectSend(req, nil)

		evt := core.NewTickEvent(10, unit)
		unit.Handle(evt)

		Expect(unit.writing).To(BeNil())
		Expect(len(engine.ScheduledEvent)).To(Equal(0))
		Expect(conn.AllExpectedSent()).To(BeTrue())
	})

})
