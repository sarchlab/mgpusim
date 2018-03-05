package driver

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/mem"
)

var _ = Describe("Driver", func() {
	var (
		storage *mem.Storage
		driver  *Driver
	)

	BeforeEach(func() {
		storage = mem.NewStorage(4 * mem.GB)
		driver = NewDriver()
	})

	It("should allocate memory", func() {
		ptr := driver.AllocateMemory(storage, 8)
		Expect(ptr).To(Equal(GPUPtr(0)))

		ptr = driver.AllocateMemory(storage, 24)
		Expect(ptr).To(Equal(GPUPtr(8)))
	})

})
