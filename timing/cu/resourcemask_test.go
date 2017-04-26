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

		offset := mask.NextRegion(20, cu.AllocStatusFree)
		Expect(offset).To(Equal(20))
	})

	It("should return negtive value if no consecutive region is found", func() {
		mask.SetStatus(0, 5, cu.AllocStatusReserved)
		mask.SetStatus(15, 80, cu.AllocStatusReserved)
		mask.SetStatus(100, 25, cu.AllocStatusReserved)

		offset := mask.NextRegion(20, cu.AllocStatusFree)
		Expect(offset).To(Equal(-1))
	})
})
