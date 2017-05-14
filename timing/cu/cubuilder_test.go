package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("Builder", func() {
	var b *Builder

	BeforeEach(func() {
		b = NewBuilder()
		b.Decoder = insts.NewDisassembler()
		b.InstMem = core.NewMockComponent("MockInstMem")
		b.ToInstMem = core.NewMockConnection()
	})

	It("should build default compute unit", func() {
		computeUnit := b.Build()

		Expect(computeUnit).NotTo(BeNil())
		Expect(computeUnit.Scheduler).NotTo(BeNil())
		Expect(len(computeUnit.VRegFiles)).To(Equal(4))
		Expect(computeUnit.SRegFile).NotTo(BeNil())
		Expect(computeUnit.Scheduler.fetchArbitor).NotTo(BeNil())
		Expect(computeUnit.Scheduler.issueArbitor).NotTo(BeNil())
		Expect(computeUnit.Scheduler.InstMem).To(BeIdenticalTo(b.InstMem))
		Expect(computeUnit.Scheduler.decoder).To(BeIdenticalTo(b.Decoder))

		Expect(computeUnit.Scheduler.GetConnection("ToInstMem")).To(
			BeIdenticalTo(b.ToInstMem))

	})
})
