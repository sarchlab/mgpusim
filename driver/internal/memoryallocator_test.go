package internal

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

var _ = Describe("MemoryAllocatorImpl", func() {

	var (
		mockCtrl  *gomock.Controller
		allocator *memoryAllocatorImpl
		mmu       *MockMMU
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mmu = NewMockMMU(mockCtrl)

		allocator = NewMemoryAllocator(mmu, 12).(*memoryAllocatorImpl)
		configAFourGPUSystem(allocator)

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should allocate memory", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x100000000,
				VAddr:    4096,
				PageSize: 4096,
				GPUID:    1,
				Valid:    true,
			})

		ptr := allocator.Allocate(1, 8, 1)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should allocate unified memory", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x100000000,
				VAddr:    4096,
				PageSize: 4096,
				GPUID:    1,
				Valid:    true,
				Unified:  true,
			})

		ptr := allocator.AllocateUnified(1, 8)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should allocate memory larger than a page", func() {
		for i := uint64(0); i < 3; i++ {
			mmu.EXPECT().CreatePage(
				&vm.Page{
					PID:      1,
					PAddr:    0x100000000 + 0x1000*i,
					VAddr:    4096 + 0x1000*i,
					GPUID:    1,
					PageSize: 4096,
					Valid:    true,
				})
		}

		ptr := allocator.Allocate(1, 8196, 1)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should remap page to another device", func() {
		page := &vm.Page{
			PID:      1,
			PAddr:    0x100000000,
			VAddr:    4096,
			PageSize: 4096,
			GPUID:    1,
			Valid:    true,
		}
		mmu.EXPECT().CreatePage(page)
		ptr := allocator.Allocate(1, 4000, 1)

		mmu.EXPECT().RemovePage(ca.PID(1), page.VAddr)
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			PAddr:    0x200000000,
			VAddr:    4096,
			PageSize: 4096,
			GPUID:    2,
			Valid:    true,
		})
		allocator.Remap(1, ptr, 4000, 2)
	})
})

func configAFourGPUSystem(allocator *memoryAllocatorImpl) {
	for i := 0; i < 5; i++ { // 5 devices = 1 CPU + 4 GPUs
		allocator.RegisterStorage(0x100000000)
	}
}
