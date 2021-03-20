package driver

import (
	"github.com/golang/mock/gomock"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/util/v2/ca"
)

var _ = ginkgo.Describe("Distributor", func() {
	var (
		ctrl         *gomock.Controller
		ctx          *Context
		memAllocator *MockMemoryAllocator
		dist         *distributorImpl
	)

	ginkgo.BeforeEach(func() {
		ctrl = gomock.NewController(ginkgo.GinkgoT())
		memAllocator = NewMockMemoryAllocator(ctrl)
		dist = newDistributorImpl(memAllocator)
		dist.pageSizeAsPowerOf2 = 12

		ctx = &Context{
			pid:          1,
			currentGPUID: 1,
		}
	})

	ginkgo.It("should distribute memory less than a page", func() {
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100000000), uint64(4096), 1)
		bytes := dist.Distribute(ctx, 0x100000000, 1024, []int{1, 2, 3})

		Expect(bytes).To(Equal([]uint64{4096, 0, 0}))
	})

	ginkgo.It("should distribute pages", func() {
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100000000), uint64(0x1000), 1)
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100001000), uint64(0x1000), 2)
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100002000), uint64(0x1000), 3)
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100003000), uint64(0x1000), 3)
		memAllocator.EXPECT().
			Remap(ca.PID(1), uint64(0x100004000), uint64(0x1000), 3)

		bytes := dist.Distribute(ctx, 0x100000000, 0x4020, []int{1, 2, 3})

		Expect(bytes).To(Equal([]uint64{4096, 4096, 12288}))
	})
})
