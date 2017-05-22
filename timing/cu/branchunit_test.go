package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("Branch Unit", func() {
	var (
		engine    *core.MockEngine
		scheduler *core.MockComponent
		unit      *BranchUnit
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		scheduler = core.NewMockComponent("scheduler")
		unit = NewBranchUnit("branch_unit", engine, scheduler)
		unit.Freq = 1
	})

	It("should not accept instruction is reading stage is occupied", func() {
		wf := new(Wavefront)
		unit.reading = wf

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).NotTo(BeNil())
	})

	It("should accept instruction is reading stage is not occupied", func() {
		wf := new(Wavefront)

		req := NewIssueInstReq(nil, unit, 10, nil, wf)
		err := unit.Recv(req)

		Expect(err).To(BeNil())
		Expect(unit.reading).To(BeIdenticalTo(wf))
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})
})
