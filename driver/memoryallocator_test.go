package driver

import (
	"github.com/golang/mock/gomock"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

var _ = ginkgo.Describe("MemoryAllocatorImpl", func() {

	var (
		mockCtrl  *gomock.Controller
		allocator *memoryAllocatorImpl
		context   *Context
		mmu       *MockMMU
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		mmu = NewMockMMU(mockCtrl)

		allocator = newMemoryAllocatorImpl(mmu)
		allocator.pageSizeAsPowerOf2 = 12
		configAFourGPUSystem(allocator)

		context = &Context{}
		context.pid = 1
		context.currentGPUID = 1
	})

	ginkgo.AfterEach(func() {
		mockCtrl.Finish()
	})

	ginkgo.It("should allocate memory", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x100000000,
				VAddr:    4096,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := allocator.Allocate(context, 8)
		Expect(ptr).To(Equal(GPUPtr(4096)))
		Expect(context.prevPageVAddr).To(Equal(uint64(4096)))

		ptr = allocator.Allocate(context, 24)
		Expect(ptr).To(Equal(GPUPtr(4104)))
		Expect(context.prevPageVAddr).To(Equal(uint64(4096)))
	})

	ginkgo.It("should allocate memory with alignment", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x100000000,
				VAddr:    4096,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := allocator.AllocateWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(4096)))
		Expect(allocator.deviceMemoryStates[1].allocatedPages).To(HaveLen(1))
		Expect(allocator.deviceMemoryStates[1].memoryChunks).To(HaveLen(2))

		ptr = allocator.AllocateWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(4160)))
		Expect(allocator.deviceMemoryStates[1].allocatedPages).To(HaveLen(1))
		Expect(allocator.deviceMemoryStates[1].memoryChunks).To(HaveLen(4))
	})

	ginkgo.It("should allocate memory larger than a page", func() {
		for i := uint64(0); i < 3; i++ {
			mmu.EXPECT().CreatePage(
				&vm.Page{
					PID:      1,
					PAddr:    0x100000000 + 0x1000*i,
					VAddr:    4096 + 0x1000*i,
					PageSize: 4096,
					Valid:    true,
				})
		}

		ptr := allocator.Allocate(context, 8196)
		Expect(ptr).To(Equal(GPUPtr(4096)))
		Expect(allocator.deviceMemoryStates[1].allocatedPages).To(HaveLen(3))
	})

	ginkgo.It("should remap page to another device", func() {
		page := &vm.Page{
			PID:      1,
			PAddr:    0x100000000,
			VAddr:    4096,
			PageSize: 4096,
			Valid:    true,
		}

		mmu.EXPECT().CreatePage(page)
		ptr := allocator.Allocate(context, 4000)

		// mmu.EXPECT().
		// 	Translate(ca.PID(1), uint64(page.VAddr)).
		// 	Return(page)
		mmu.EXPECT().RemovePage(ca.PID(1), uint64(page.VAddr))
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			PAddr:    0x200000000,
			VAddr:    4096,
			PageSize: 4096,
			Valid:    true,
		})
		allocator.Remap(context, uint64(ptr), 4096, 2)

		Expect(allocator.deviceMemoryStates[1].allocatedPages).To(HaveLen(0))
		Expect(allocator.deviceMemoryStates[2].allocatedPages).To(HaveLen(1))
		Expect(allocator.deviceMemoryStates[2].allocatedPages[0].VAddr).
			To(Equal(uint64(ptr)))
		Expect(allocator.deviceMemoryStates[2].allocatedPages[0].PAddr).
			To(Equal(uint64(0x200000000)))
		Expect(allocator.deviceMemoryStates[1].memoryChunks).To(HaveLen(0))
		Expect(allocator.deviceMemoryStates[2].memoryChunks).To(HaveLen(2))
	})
})

func configAFourGPUSystem(allocator *memoryAllocatorImpl) {
	for i := 0; i < 5; i++ { // 5 devices = 1 CPU + 4 GPUs
		allocator.RegisterStorage(0x100000000)
	}

}
