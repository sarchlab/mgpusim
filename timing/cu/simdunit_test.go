package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("SIMD Unit", func() {

	var (
		engine *core.MockEngine
		conn   *core.MockConnection
		unit   *SIMDUnit
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		conn = core.NewMockConnection()
		unit = NewSIMDUnit("simd", engine, nil)
		unit.Freq = 1

		core.PlugIn(unit, "ToScheduler", conn)
	})

	It("should not accept instruction if there is another instruction is waiting", func() {
		wf := new(Wavefront)
		unit.reading = wf

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).NotTo(BeNil())
	})

	It("should accept instruction if readWaiting is not occupied", func() {
		unit.reading = nil

		wf := new(Wavefront)
		wf.CompletedLanes = 64 // From last execution
		req := NewIssueInstReq(nil, unit, 10, nil, wf)

		err := unit.Recv(req)
		Expect(err).To(BeNil())
		Expect(unit.reading).To(BeIdenticalTo(wf))
		Expect(wf.CompletedLanes).To(Equal(0))

	})

	It("should move instruction from read to exec", func() {
		wf := new(Wavefront)

		unit.reading = wf
		unit.executing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.executing).To(BeIdenticalTo(wf))
		Expect(unit.reading).To(BeNil())
	})

	It("should stay in exec stage if not completed", func() {
		wf := new(Wavefront)
		wf.CompletedLanes = 16

		unit.reading = nil
		unit.executing = wf
		unit.writing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.writing).To(BeNil())
		Expect(unit.executing).To(BeIdenticalTo(wf))
		Expect(unit.executing.CompletedLanes).To(Equal(32))
	})

	It("should move instruction from exec to write", func() {
		wf := new(Wavefront)
		wf.CompletedLanes = 48

		unit.reading = nil
		unit.executing = wf
		unit.writing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.writing).To(BeIdenticalTo(wf))
		Expect(unit.executing).To(BeNil())
		Expect(wf.CompletedLanes).To(Equal(64))
	})

	It("should move instruction from write to write done", func() {
		wf := new(Wavefront)

		unit.writing = wf
		unit.writeDone = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.writing).To(BeNil())
		Expect(unit.writeDone).To(BeIdenticalTo(wf))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(engine.ScheduledEvent[0].Time()).To(BeNumerically("~", 10.5, 1e-12))
	})

	It("should handle deferred send", func() {
		wf := new(Wavefront)
		unit.writeDone = wf
		req := NewInstCompletionReq(unit, nil, 10.5, wf)
		deferredSend := core.NewDeferredSend(req)

		conn.ExpectSend(req, nil)

		unit.Handle(deferredSend)

		Expect(unit.writeDone).To(BeNil())
		Expect(conn.AllExpectedSent()).To(BeTrue())
	})

})
