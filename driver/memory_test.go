package driver

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/akita"
)

var _ = Describe("Driver", func() {
	var (
		storage *mem.Storage
		driver  *Driver
		mmu     *vm.MMUImpl
		engine *akita.MockEngine
	)

	BeforeEach(func() {
		storage = mem.NewStorage(4 * mem.GB)
		driver = NewDriver(nil)
		engine = akita.NewMockEngine()
		mmu = vm.NewMMU("mmu", engine)
	})

	It("should allocate memory", func() {
		ptr := driver.AllocateMemory(storage, 8,mmu,4096)
		Expect(ptr).To(Equal(GPUPtr(0x100000000)))

		ptr = driver.AllocateMemory(storage, 24,mmu,4096)
		Expect(ptr).To(Equal(GPUPtr(0x100000008)))
	})

	It("should allocate memory with alignment", func() {
		driver.AllocateMemory(storage, 8,mmu,4096)

		ptr := driver.AllocateMemoryWithAlignment(storage, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(64)))

		ptr = driver.AllocateMemory(storage, 8,mmu,4096)
		Expect(ptr).To(Equal(GPUPtr(0x100000008)))
	})

	It("should free memory", func() {
		Expect(func() { driver.FreeMemory(storage, 0) }).To(Panic())

		ptr := driver.AllocateMemory(storage, 4,mmu,4096)
		ptr2 := driver.AllocateMemory(storage, 16,mmu,4096)
		ptr3 := driver.AllocateMemory(storage, 8,mmu,4096)
		ptr4 := driver.AllocateMemory(storage, 12,mmu,4096)
		ptr5 := driver.AllocateMemory(storage, 24,mmu,4096)

		driver.memoryMasks[storage].Chunks[5].ByteSize = 36

		driver.FreeMemory(storage, ptr)
		Expect(driver.memoryMasks[storage].Chunks[0].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[storage].Chunks[0].ByteSize).To(Equal(uint64(4)))

		driver.FreeMemory(storage, ptr2)
		Expect(driver.memoryMasks[storage].Chunks[0].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[storage].Chunks[0].ByteSize).To(Equal(uint64(20)))

		driver.FreeMemory(storage, ptr5)
		Expect(driver.memoryMasks[storage].Chunks[3].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[storage].Chunks[3].ByteSize).To(Equal(uint64(60)))

		driver.FreeMemory(storage, ptr4)
		Expect(driver.memoryMasks[storage].Chunks[2].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[storage].Chunks[2].ByteSize).To(Equal(uint64(72)))

		driver.FreeMemory(storage, ptr3)
		Expect(driver.memoryMasks[storage].Chunks[0].Occupied).To(Equal(false))
		Expect(len(driver.memoryMasks[storage].Chunks)).To(Equal(1))
	})

})
