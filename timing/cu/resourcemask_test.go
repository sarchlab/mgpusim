package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

var _ = Describe("Resource Mask", func() {

	var mask *cu.ResourceMask

	BeforeEach(func() {
		mask = cu.NewResourceMask(128)
	})

	It("should get the first region that has the required status", func() {
		mask.SetStatus(0, 5, cu.AllocStatusReserved)
		mask.SetStatus(15, 5, cu.AllocStatusReserved)

		offset, ok := mask.NextRegion(20, cu.AllocStatusFree)
		Expect(offset).To(Equal(20))
		Expect(ok).To(BeTrue())
	})

	It("should return negtive value if no consecutive region is found", func() {
		mask.SetStatus(0, 5, cu.AllocStatusReserved)
		mask.SetStatus(15, 80, cu.AllocStatusReserved)
		mask.SetStatus(100, 25, cu.AllocStatusReserved)

		offset, ok := mask.NextRegion(20, cu.AllocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeFalse())
	})

	It("should always return offset 0 and ok if input length is 0", func() {
		mask.SetStatus(0, 5, cu.AllocStatusReserved)
		offset, ok := mask.NextRegion(0, cu.AllocStatusFree)
		Expect(offset).To(Equal(0))
		Expect(ok).To(BeTrue())
	})

	It("should be able to convert status", func() {
		mask.SetStatus(0, 5, cu.AllocStatusToReserve)
		mask.SetStatus(10, 20, cu.AllocStatusToReserve)

		mask.ConvertStatus(cu.AllocStatusToReserve, cu.AllocStatusReserved)

		offset, ok := mask.NextRegion(20, cu.AllocStatusReserved)
		Expect(offset).To(Equal(10))
		Expect(ok).To(BeTrue())
	})

	It("should be able to get the element count of a certain status", func() {
		mask.SetStatus(0, 5, cu.AllocStatusToReserve)
		mask.SetStatus(10, 20, cu.AllocStatusReserved)

		Expect(mask.StatusCount(cu.AllocStatusToReserve)).To(Equal(5))
		Expect(mask.StatusCount(cu.AllocStatusReserved)).To(Equal(20))
	})
})
