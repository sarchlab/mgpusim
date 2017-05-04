package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

var _ = Describe("Builder", func() {
	var b *cu.Builder

	BeforeEach(func() {
		b = cu.NewBuilder()
	})

	It("should build default compute unit", func() {
		computeUnit := b.Build()

		Expect(computeUnit).NotTo(BeNil())
		Expect(computeUnit.Scheduler).NotTo(BeNil())
		Expect(len(computeUnit.VRegFiles)).To(Equal(4))
		Expect(computeUnit.SRegFile).NotTo(BeNil())
	})
})
