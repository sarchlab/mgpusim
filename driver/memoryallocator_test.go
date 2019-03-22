package driver

import (
	"github.com/golang/mock/gomock"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mock_vm"
)

var _ = ginkgo.Describe("MemoryallocatorImpl", func() {

	var (
		mockCtrl  *gomock.Controller
		allocator *memoryAllocatorImpl
		context   *Context
		mmu       *mock_vm.MockMMU
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		mmu = mock_vm.NewMockMMU(mockCtrl)

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
				VAddr:    0x200000000,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := allocator.Allocate(context, 8)
		Expect(ptr).To(Equal(GPUPtr(0x200000000)))

		ptr = allocator.Allocate(context, 24)
		Expect(ptr).To(Equal(GPUPtr(0x200000008)))
	})

	ginkgo.It("should allocate memory with alignment", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x100000000,
				VAddr:    0x200000000,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := allocator.AllocateWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(0x200000000)))
		Expect(allocator.allocatedPages[1]).To(HaveLen(1))
		Expect(allocator.memoryMasks[1]).To(HaveLen(2))

		ptr = allocator.AllocateWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(0x200000040)))
		Expect(allocator.allocatedPages[1]).To(HaveLen(1))
		Expect(allocator.memoryMasks[1]).To(HaveLen(4))
	})

	ginkgo.It("should allocate memory larger than a page", func() {
		for i := uint64(0); i < 3; i++ {
			mmu.EXPECT().CreatePage(
				&vm.Page{
					PID:      1,
					PAddr:    0x100000000 + 0x1000*i,
					VAddr:    0x200000000 + 0x1000*i,
					PageSize: 4096,
					Valid:    true,
				})
		}

		ptr := allocator.Allocate(context, 8196)
		Expect(ptr).To(Equal(GPUPtr(0x200000000)))
		Expect(allocator.allocatedPages[1]).To(HaveLen(3))
	})

	ginkgo.It("should remap page to another device", func() {
		page := &vm.Page{
			PID:      1,
			PAddr:    0x100000000,
			VAddr:    0x200000000,
			PageSize: 4096,
			Valid:    true,
		}

		mmu.EXPECT().CreatePage(page)
		ptr := allocator.Allocate(context, 4000)

		// mmu.EXPECT().
		// 	Translate(vm.PID(1), uint64(page.VAddr)).
		// 	Return(page)
		mmu.EXPECT().RemovePage(vm.PID(1), uint64(page.VAddr))
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			PAddr:    0x200000000,
			VAddr:    0x200000000,
			PageSize: 4096,
			Valid:    true,
		})
		allocator.Remap(context, uint64(ptr), 4096, 2)

		Expect(allocator.allocatedPages[1]).To(HaveLen(0))
		Expect(allocator.allocatedPages[2]).To(HaveLen(1))
		Expect(allocator.allocatedPages[2][0].VAddr).
			To(Equal(uint64(ptr)))
		Expect(allocator.allocatedPages[2][0].PAddr).
			To(Equal(uint64(0x200000000)))
		Expect(allocator.memoryMasks[1]).To(HaveLen(0))
		Expect(allocator.memoryMasks[2]).To(HaveLen(2))
	})
})

func configAFourGPUSystem(allocator *memoryAllocatorImpl) {
	for i := 0; i < 5; i++ { // 5 devices = 1 CPU + 4 GPUs
		allocator.RegisterStorage(0x100000000)
	}

}
