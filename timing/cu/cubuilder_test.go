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
		b.SIMDCount = 4
		b.Decoder = insts.NewDisassembler()
		b.InstMem = core.NewMockComponent("MockInstMem")
		b.ToInstMem = core.NewMockConnection()
	})

	It("should build default compute unit", func() {
		computeUnit := b.Build()

		Expect(computeUnit).NotTo(BeNil())

		Expect(len(computeUnit.VRegFiles)).To(Equal(4))
		Expect(computeUnit.SRegFile).NotTo(BeNil())

		expectSchedulerSet(computeUnit, b)
		expectDecodersSet(computeUnit, b)

		Expect(len(computeUnit.SIMDUnits)).To(Equal(4))
		Expect(computeUnit.BranchUnit).NotTo(BeNil())
		Expect(computeUnit.ScalarUnit).NotTo(BeNil())
		Expect(computeUnit.LDSUnit).NotTo(BeNil())
		Expect(computeUnit.VMemUnit).NotTo(BeNil())

		Expect(computeUnit.Scheduler.GetConnection("ToInstMem")).To(
			BeIdenticalTo(b.ToInstMem))

	})
})

func expectSchedulerSet(computeUnit *ComputeUnit, b *Builder) {
	Expect(computeUnit.Scheduler).NotTo(BeNil())
	scheduler := computeUnit.Scheduler.(*Scheduler)
	Expect(scheduler.fetchArbitor).NotTo(BeNil())
	Expect(scheduler.issueArbitor).NotTo(BeNil())
	Expect(scheduler.InstMem).To(BeIdenticalTo(b.InstMem))
	Expect(scheduler.decoder).To(BeIdenticalTo(b.Decoder))
}

func expectDecodersSet(computeUnit *ComputeUnit, b *Builder) {
	Expect(computeUnit.VectorDecode).NotTo(BeNil())
	vectorDecode := computeUnit.VectorDecode.(*VectorDecodeUnit)
	Expect(vectorDecode.SIMDUnits).To(Equal(computeUnit.SIMDUnits))

	Expect(computeUnit.VMemDecode).NotTo(BeNil())
	vMemDecode := computeUnit.VMemDecode.(*SimpleDecodeUnit)
	Expect(vMemDecode.ExecUnit).To(BeIdenticalTo(computeUnit.VMemUnit))

	Expect(computeUnit.ScalarDecode).NotTo(BeNil())
	Expect(computeUnit.LDSDecode).NotTo(BeNil())
}
