package resource

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource Mask", func() {

	var mask *resourceMaskImpl

	BeforeEach(func() {
		mask = newResourceMask(128)
	})

	It("should get the first region that has the required status", func() {
		mask.setStatus(0, 5, allocStatusReserved)
		mask.setStatus(15, 5, allocStatusReserved)

		offset, ok := mask.nextRegion(20, allocStatusFree)
		Expect(offset).To(Equal(20))
		Expect(ok).To(BeTrue())
	})

	It("should return negtive value if no consecutive region is found", func() {
		mask.setStatus(0, 5, allocStatusReserved)
		mask.setStatus(15, 80, allocStatusReserved)
		mask.setStatus(100, 25, allocStatusReserved)

		offset, ok := mask.nextRegion(20, allocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeFalse())
	})

	It("should always return offset 0 and ok if input length is 0", func() {
		mask.setStatus(0, 5, allocStatusReserved)
		offset, ok := mask.nextRegion(0, allocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeTrue())
	})

	It("should be able to convert status", func() {
		mask.setStatus(0, 5, allocStatusToReserve)
		mask.setStatus(10, 20, allocStatusToReserve)

		mask.convertStatus(allocStatusToReserve, allocStatusReserved)

		offset, ok := mask.nextRegion(20, allocStatusReserved)
		Expect(offset).To(Equal(10))
		Expect(ok).To(BeTrue())
	})

	It("should be able to get the element count of a certain status", func() {
		mask.setStatus(0, 5, allocStatusToReserve)
		mask.setStatus(10, 20, allocStatusReserved)

		Expect(mask.statusCount(allocStatusToReserve)).To(Equal(5))
		Expect(mask.statusCount(allocStatusReserved)).To(Equal(20))
	})
})
