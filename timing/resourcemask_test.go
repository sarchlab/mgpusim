package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource Mask", func() {

	var mask *ResourceMask

	BeforeEach(func() {
		mask = NewResourceMask(128)
	})

	It("should get the first region that has the required status", func() {
		mask.SetStatus(0, 5, AllocStatusReserved)
		mask.SetStatus(15, 5, AllocStatusReserved)

		offset, ok := mask.NextRegion(20, AllocStatusFree)
		Expect(offset).To(Equal(20))
		Expect(ok).To(BeTrue())
	})

	It("should return negtive value if no consecutive region is found", func() {
		mask.SetStatus(0, 5, AllocStatusReserved)
		mask.SetStatus(15, 80, AllocStatusReserved)
		mask.SetStatus(100, 25, AllocStatusReserved)

		offset, ok := mask.NextRegion(20, AllocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeFalse())
	})

	It("should always return offset 0 and ok if input length is 0", func() {
		mask.SetStatus(0, 5, AllocStatusReserved)
		offset, ok := mask.NextRegion(0, AllocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeTrue())
	})

	It("should be able to convert status", func() {
		mask.SetStatus(0, 5, AllocStatusToReserve)
		mask.SetStatus(10, 20, AllocStatusToReserve)

		mask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)

		offset, ok := mask.NextRegion(20, AllocStatusReserved)
		Expect(offset).To(Equal(10))
		Expect(ok).To(BeTrue())
	})

	It("should be able to get the element count of a certain status", func() {
		mask.SetStatus(0, 5, AllocStatusToReserve)
		mask.SetStatus(10, 20, AllocStatusReserved)

		Expect(mask.StatusCount(AllocStatusToReserve)).To(Equal(5))
		Expect(mask.StatusCount(AllocStatusReserved)).To(Equal(20))
	})
})
