package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Builder", func() {
	var b *Builder

	BeforeEach(func() {
		b = NewBuilder()
	})

	It("should build default compute unit", func() {
		computeUnit := b.Build()

		Expect(computeUnit).NotTo(BeNil())
		Expect(computeUnit.Scheduler).NotTo(BeNil())
		Expect(len(computeUnit.VRegFiles)).To(Equal(4))
		Expect(computeUnit.SRegFile).NotTo(BeNil())
	})
})
