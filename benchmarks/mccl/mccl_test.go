package mccl_test

import (
	"log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v2/benchmarks/mccl"
	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/samples/runner"
)

var _ = Describe("MCCL", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		gpuIDs    []int
		comms     []*mccl.Communicator
	)

	BeforeEach(func() {
		platform := runner.MakeR9NanoBuilder().
			WithNumGPU(4).
			Build()
		gpuDriver = platform.Driver
		gpuDriver.Run()
		context = gpuDriver.Init()
	})

	AfterEach(func() {
		gpuDriver.Terminate()
	})

	It("Broadcast Test", func() {
		gpuNum := 4
		dataSize := 1024
		root := 1
		datas := make([]driver.GPUPtr, gpuNum)

		for i := 0; i < gpuNum; i++ {
			gpuDriver.SelectGPU(context, i+1)
			data := gpuDriver.AllocateMemory(context, uint64(dataSize*4))
			gpuIDs = append(gpuIDs, i+1)
			datas[i] = data
		}

		tmp := make([]float32, uint64(dataSize))
		for j := 0; j < dataSize; j++ {
			tmp[j] = -0.0000123
		}

		gpuDriver.SelectGPU(context, root)
		gpuDriver.MemCopyH2D(context, datas[root-1], tmp)
		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		mccl.BroadcastRing(gpuDriver, comms, root, datas, dataSize)

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			gpuDriver.MemCopyD2H(context, tmp, datas[i])
			for j := 0; j < dataSize; j++ {
				Expect(tmp[i]).To(Equal(float32(-0.0000123)))
			}
		}
	})

	It("AllReduce Test Buffer Size < Data Size", func() {
		gpuNum := 4
		var dataSize uint32 = 1029
		var bufSize uint32 = 256
		datas := make([]driver.GPUPtr, gpuNum)
		bufs := make([]driver.GPUPtr, gpuNum)
		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			for j := 0; j < int(dataSize); j++ {
				tmp[j] = float32(i + 1)
			}
			gpuDriver.SelectGPU(context, i+1)
			data := gpuDriver.AllocateMemory(context, uint64(dataSize*4))
			gpuDriver.MemCopyH2D(context, data, tmp)
			buf := gpuDriver.AllocateMemory(context, uint64(bufSize*4))
			gpuIDs = append(gpuIDs, i+1)
			datas[i] = data
			bufs[i] = buf
		}

		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		mccl.AllReduceRing(
			gpuDriver, comms, datas, int(dataSize), bufs, int(bufSize))

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			gpuDriver.MemCopyD2H(context, tmp, datas[i])
			for j := 0; j < int(dataSize); j++ {
				Expect(tmp[i]).To(Equal(float32(2.5)))
			}
			log.Printf("Passed")
		}
	})

	It("AllReduce Test Buffer Size == Data Size", func() {
		gpuNum := 4
		var dataSize uint32 = 1029
		var bufSize uint32 = 1029
		datas := make([]driver.GPUPtr, gpuNum)
		bufs := make([]driver.GPUPtr, gpuNum)
		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			for j := 0; j < int(dataSize); j++ {
				tmp[j] = float32(i + 1)
			}
			gpuDriver.SelectGPU(context, i+1)
			data := gpuDriver.AllocateMemory(context, uint64(dataSize*4))
			gpuDriver.MemCopyH2D(context, data, tmp)
			buf := gpuDriver.AllocateMemory(context, uint64(bufSize*4))
			gpuIDs = append(gpuIDs, i+1)
			datas[i] = data
			bufs[i] = buf
		}

		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		mccl.AllReduceRing(
			gpuDriver, comms, datas, int(dataSize), bufs, int(bufSize))

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			gpuDriver.MemCopyD2H(context, tmp, datas[i])
			for j := 0; j < int(dataSize); j++ {
				Expect(tmp[i]).To(Equal(float32(2.5)))
			}
			log.Printf("Passed")
		}
	})

})
