package timing

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

		expectSchedulerSet(computeUnit, b)
		expectDecodersSet(computeUnit, b)
		expectExecUnitsSet(computeUnit, b)
	})
})

func expectRegisterFilesSet(computeUnit *ComputeUnit) {
	Expect(len(computeUnit.VRegFiles)).To(Equal(4))
	Expect(computeUnit.SRegFile).NotTo(BeNil())
}

func expectSchedulerSet(computeUnit *ComputeUnit, b *Builder) {
	Expect(computeUnit.Scheduler).NotTo(BeNil())
	scheduler := computeUnit.Scheduler.(*Scheduler)
	Expect(scheduler.fetchArbiter).NotTo(BeNil())
	Expect(scheduler.issueArbiter).NotTo(BeNil())
	Expect(scheduler.InstMem).To(BeIdenticalTo(b.InstMem))
	Expect(scheduler.decoder).To(BeIdenticalTo(b.Decoder))
	Expect(scheduler.LDSDecoder).To(BeIdenticalTo(computeUnit.LDSDecode))
	Expect(scheduler.ScalarDecoder).To(BeIdenticalTo(computeUnit.ScalarDecode))
	Expect(scheduler.VectorDecoder).To(BeIdenticalTo(computeUnit.VectorDecode))
	Expect(scheduler.VectorMemDecoder).To(BeIdenticalTo(computeUnit.VMemDecode))
	Expect(scheduler.BranchUnit).To(BeIdenticalTo(computeUnit.BranchUnit))
	Expect(computeUnit.Scheduler.GetConnection("ToInstMem")).To(
		BeIdenticalTo(b.ToInstMem))
}

func expectDecodersSet(computeUnit *ComputeUnit, b *Builder) {
	Expect(computeUnit.VectorDecode).NotTo(BeNil())
	vectorDecode := computeUnit.VectorDecode.(*VectorDecodeUnit)
	Expect(vectorDecode.SIMDUnits).To(Equal(computeUnit.SIMDUnits))

	Expect(computeUnit.VMemDecode).NotTo(BeNil())
	vMemDecode := computeUnit.VMemDecode.(*SimpleDecodeUnit)
	Expect(vMemDecode.ExecUnit).To(BeIdenticalTo(computeUnit.VMemUnit))

	Expect(computeUnit.ScalarDecode).NotTo(BeNil())
	scalarDecode := computeUnit.ScalarDecode.(*SimpleDecodeUnit)
	Expect(scalarDecode.ExecUnit).To(BeIdenticalTo(computeUnit.ScalarUnit))

	Expect(computeUnit.LDSDecode).NotTo(BeNil())
	ldsDecode := computeUnit.LDSDecode.(*SimpleDecodeUnit)
	Expect(ldsDecode.ExecUnit).To(BeIdenticalTo(computeUnit.LDSUnit))
}

func expectExecUnitsSet(computeUnit *ComputeUnit, b *Builder) {
	Expect(len(computeUnit.SIMDUnits)).To(Equal(4))
	Expect(computeUnit.BranchUnit).NotTo(BeNil())
	Expect(computeUnit.ScalarUnit).NotTo(BeNil())
	Expect(computeUnit.LDSUnit).NotTo(BeNil())
	Expect(computeUnit.VMemUnit).NotTo(BeNil())
}
