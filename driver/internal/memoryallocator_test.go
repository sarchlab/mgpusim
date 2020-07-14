package internal

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem/vm"
)

var _ = Describe("MemoryAllocatorImpl", func() {

	var (
		mockCtrl  *gomock.Controller
		allocator *memoryAllocatorImpl
		pageTable *MockPageTable
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		pageTable = NewMockPageTable(mockCtrl)

		allocator = NewMemoryAllocator(pageTable, 12).(*memoryAllocatorImpl)
		configAFourGPUSystem(allocator)

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should allocate memory", func() {
		pageTable.EXPECT().Insert(
			vm.Page{
				PID:      1,
				PAddr:    0x1_0000_1000,
				VAddr:    4096,
				PageSize: 4096,
				GPUID:    1,
				Valid:    true,
			})

		ptr := allocator.Allocate(1, 8, 1)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should allocate unified memory", func() {
		pageTable.EXPECT().Insert(
			vm.Page{
				PID:      1,
				PAddr:    0x1_0000_1000,
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
			pageTable.EXPECT().Insert(
				vm.Page{
					PID:      1,
					PAddr:    0x1_0000_1000 + 0x1000*i,
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
		page := vm.Page{
			PID:      1,
			PAddr:    0x1_0000_1000,
			VAddr:    4096,
			PageSize: 4096,
			GPUID:    1,
			Valid:    true,
		}
		pageTable.EXPECT().Insert(page)
		ptr := allocator.Allocate(1, 4000, 1)

		updatedPage := page
		updatedPage.PAddr = 0x2_0000_1000
		updatedPage.GPUID = 2
		pageTable.EXPECT().Update(updatedPage)
		allocator.Remap(1, ptr, 4000, 2)
	})
})

func configAFourGPUSystem(allocator *memoryAllocatorImpl) {
	cpu := &Device{
		ID:       0,
		Type:     DeviceTypeCPU,
		MemState: NewDeviceMemoryState(12),
	}
	cpu.SetTotalMemSize(0x1_0000_0000)
	allocator.RegisterDevice(cpu)

	for i := 0; i < 4; i++ { // 5 devices = 1 CPU + 4 GPUs
		gpu := &Device{
			ID:       i + 1,
			Type:     DeviceTypeGPU,
			MemState: NewDeviceMemoryState(12),
		}
		gpu.SetTotalMemSize(0x1_0000_0000)
		allocator.RegisterDevice(gpu)
	}
}
