package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("SIMD Unit", func() {

	var (
		engine *core.MockEngine
		unit   *SIMDUnit
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		unit = NewSIMDUnit("simd", engine, nil)
		unit.Freq = 1
	})

	It("should not accept instruction if there is another instruction is waiting", func() {
		wf := new(Wavefront)
		unit.readWaiting = wf

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).NotTo(BeNil())
	})

	It("should accept instruction if readWaiting is not occupied", func() {
		unit.readWaiting = nil

		wf := new(Wavefront)
		req := NewIssueInstReq(nil, unit, 10, nil, wf)

		err := unit.Recv(req)
		Expect(err).To(BeNil())
		Expect(unit.readWaiting).To(BeIdenticalTo(wf))
	})

	It("should move instruction from read to exec", func() {
		wf := new(Wavefront)

		unit.readWaiting = nil
		unit.reading = wf
		unit.executing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.executing).To(BeIdenticalTo(wf))
		Expect(unit.reading).To(BeNil())
	})

	It("should move instruction from exec to write", func() {
		wf := new(Wavefront)
		wf.CompletedLanes = 16

		unit.reading = nil
		unit.executing = wf
		unit.writing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.writing).To(BeIdenticalTo(wf))
		Expect(unit.executing).To(BeNil())
		Expect(wf.CompletedLanes).To(Equal(32))
	})

	It("should send InstCompletionReq if instruction completed", func() {

	})

	It("should move instruction from write to read again if inst not done", func() {
		wf := new(Wavefront)

		unit.reading = nil
		unit.executing = wf
		unit.writing = nil

		unit.Handle(core.NewTickEvent(10, unit))

		Expect(unit.writing).To(BeIdenticalTo(wf))
		Expect(unit.executing).To(BeNil())
	})

})
