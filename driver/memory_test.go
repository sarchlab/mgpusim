package driver

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
)

var _ = Describe("Driver", func() {
	var (
		storage *mem.Storage
		driver  *Driver
		mmu     *vm.MMUImpl
		engine  *akita.MockEngine
	)

	BeforeEach(func() {
		storage = mem.NewStorage(4 * mem.GB)
		driver = NewDriver(nil)
		driver.registerStorage(storage, 0, 4*mem.GB)
	})

	It("should allocate memory", func() {
		ptr := driver.AllocateMemory(8)
		Expect(ptr).To(Equal(GPUPtr(0)))

		ptr = driver.AllocateMemory(24)
		Expect(ptr).To(Equal(GPUPtr(8)))
	})

	It("should allocate memory with alignment", func() {
		driver.AllocateMemory(8)

		ptr := driver.AllocateMemoryWithAlignment(8, 64)
		Expect(ptr).To(Equal(GPUPtr(64)))

		ptr = driver.AllocateMemory(8)
		Expect(ptr).To(Equal(GPUPtr(8)))
	})

	It("should free memory", func() {
		ptr := driver.AllocateMemory(4)
		ptr2 := driver.AllocateMemory(16)
		ptr3 := driver.AllocateMemory(8)
		ptr4 := driver.AllocateMemory(12)
		ptr5 := driver.AllocateMemory(24)

		driver.memoryMasks[0].Chunks[5].ByteSize = 36

		driver.FreeMemory(ptr)
		Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[0].Chunks[0].ByteSize).To(Equal(uint64(4)))

		driver.FreeMemory(ptr2)
		Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[0].Chunks[0].ByteSize).To(Equal(uint64(20)))

		driver.FreeMemory(ptr5)
		Expect(driver.memoryMasks[0].Chunks[3].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[0].Chunks[3].ByteSize).To(Equal(uint64(60)))

		driver.FreeMemory(ptr4)
		Expect(driver.memoryMasks[0].Chunks[2].Occupied).To(Equal(false))
		Expect(driver.memoryMasks[0].Chunks[2].ByteSize).To(Equal(uint64(72)))

		driver.FreeMemory(ptr3)
		Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		Expect(len(driver.memoryMasks[0].Chunks)).To(Equal(1))
	})

})
