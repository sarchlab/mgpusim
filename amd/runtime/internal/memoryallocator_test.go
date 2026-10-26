package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
)

var _ = Describe("MemoryAllocatorImpl", func() {
	var (
		allocator *memoryAllocatorImpl
		pageTable *vm.PageTable
	)

	BeforeEach(func() {
		pageTable = vm.NewPageTable(12)

		allocator = NewMemoryAllocator(pageTable, 12).(*memoryAllocatorImpl)
		configAFourGPUSystem(allocator)

	})

	AfterEach(func() {
	})

	checkPage := func(want vm.Page) {
		DeferCleanup(func() {
			got, found := pageTable.Find(want.PID, want.VAddr)
			Expect(found).To(BeTrue())
			Expect(got).To(Equal(want))
		})
	}

	It("should allocate memory", func() {
		checkPage(
			vm.Page{
				PID:      1,
				PAddr:    0x1_0000_1000,
				VAddr:    4096,
				PageSize: 4096,
				DeviceID: 1,
				Valid:    true,
			})

		ptr := allocator.Allocate(1, 8, 1)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should allocate unified memory", func() {
		checkPage(
			vm.Page{
				PID:      1,
				PAddr:    0x1_0000_1000,
				VAddr:    4096,
				PageSize: 4096,
				DeviceID: 1,
				Valid:    true,
				Unified:  true,
			})

		ptr := allocator.AllocateUnified(1, 8)
		Expect(ptr).To(Equal(uint64(4096)))
	})

	It("should allocate memory larger than a page", func() {
		for i := uint64(0); i < 3; i++ {
			checkPage(
				vm.Page{
					PID:      1,
					PAddr:    0x1_0000_1000 + 0x1000*i,
					VAddr:    4096 + 0x1000*i,
					DeviceID: 1,
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
			DeviceID: 1,
			Valid:    true,
		}
		ptr := allocator.Allocate(1, 4000, 1)
		got, found := pageTable.Find(1, ptr)
		Expect(found).To(BeTrue())
		Expect(got).To(Equal(page))

		updatedPage := page
		updatedPage.PAddr = 0x2_0000_1000
		updatedPage.DeviceID = 2
		allocator.Remap(1, ptr, 4000, 2)
		got, found = pageTable.Find(1, ptr)
		Expect(found).To(BeTrue())
		Expect(got).To(Equal(updatedPage))
	})
})

func configAFourGPUSystem(allocator *memoryAllocatorImpl) {
	cpu := &Device{
		ID:       0,
		Type:     DeviceTypeCPU,
		MemState: NewDeviceMemoryState(12, AllocatorTypeDefault),
	}
	cpu.SetTotalMemSize(0x1_0000_0000)
	allocator.RegisterDevice(cpu)

	for i := 0; i < 4; i++ { // 5 devices = 1 CPU + 4 GPUs
		gpu := &Device{
			ID:       i + 1,
			Type:     DeviceTypeGPU,
			MemState: NewDeviceMemoryState(12, AllocatorTypeDefault),
		}
		gpu.SetTotalMemSize(0x1_0000_0000)
		allocator.RegisterDevice(gpu)
	}
}
