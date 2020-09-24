package driver_test

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = ginkgo.Describe("Test Memory Copy D2", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
	)

	ginkgo.BeforeEach(func() {
		_, gpuDriver = platform.MakeEmuBuilder().
			WithNumGPU(1).
			WithoutProgressBar().
			Build()
		gpuDriver.Run()
		context = gpuDriver.Init()

	})

	ginkgo.AfterEach(func() {
		gpuDriver.Terminate()
	})

	ginkgo.It("should copy d2d", func() {
		ptr1 := gpuDriver.AllocateMemory(context, uint64(48))
		ptr2 := gpuDriver.AllocateMemory(context, uint64(48))
		hInput := make([]float32, 48)
		hOutput := make([]float32, 48)
		for i := 0; i < 48; i++ {
			hInput[i] = float32(i)
		}
		gpuDriver.MemCopyH2D(context, ptr1, hInput)
		gpuDriver.MemCopyD2D(context, ptr2, ptr1, 48*4) // 4 is the size of float32.
		gpuDriver.MemCopyD2H(context, hOutput, ptr2)
		for i := 0; i < 48; i++ {
			Expect(hInput[i] == hOutput[i])
		}
	})

	ginkgo.It("should copy d2d", func() {
		ptr1 := gpuDriver.AllocateMemory(context, uint64(49))
		ptr2 := gpuDriver.AllocateMemory(context, uint64(49))
		hInput := make([]float32, 49)
		hOutput := make([]float32, 49)
		for i := 0; i < 49; i++ {
			hInput[i] = float32(i)
		}
		gpuDriver.MemCopyH2D(context, ptr1, hInput)
		gpuDriver.MemCopyD2D(context, ptr2, ptr1, 49*4) // 4 is the size of float32.
		gpuDriver.MemCopyD2H(context, hOutput, ptr2)
		for i := 0; i < 49; i++ {
			Expect(hInput[i] == hOutput[i])
		}
	})
})
