package mccl_test

import (
	"encoding/binary"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/mccl"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

var _ = Describe("MCCL", func() {
	var (
		s         *simulation.Simulation
		gpuDriver *gpuruntime.Runtime
		context   gpuruntime.Context
		gpuIDs    []int
		comms     []*mccl.Communicator
	)

	drain := func() {
		for _, c := range s.Components() {
			if d, ok := c.(*driver.Comp); ok {
				d.TickLater()
			}
		}
		Expect(s.Run()).To(Succeed())
	}
	upload := func(ctx gpuruntime.Context, p gpuruntime.Ptr, data any) {
		q := gpuDriver.CreateCommandQueue(ctx)
		benchmarks.Upload(gpuDriver, q, p, data)
		drain()
	}
	read := func(ctx gpuruntime.Context, data any, p gpuruntime.Ptr) {
		q := gpuDriver.CreateCommandQueue(ctx)
		id := benchmarks.Download(gpuDriver, q, p, binary.Size(data))
		drain()
		Expect(benchmarks.Read(gpuDriver, id, data)).To(Succeed())
	}
	collective := func(c *mccl.Collective) {
		for !c.IsEnded() {
			Expect(c.Step(gpuDriver)).To(Succeed())
			drain()
		}
	}
	BeforeEach(func() {
		s = simulation.MakeBuilder().Build()
		gpuDriver = emusystem.MakeBuilder().
			WithSimulation(s).
			WithNumGPUs(4).
			Build()
		Expect(s.Initialize()).To(Succeed())
		context = gpuDriver.Init()
	})

	AfterEach(func() {
		s.Terminate()
	})

	It("Broadcast Test", func() {
		gpuNum := 4
		dataSize := 1024
		root := 1
		datas := make([]gpuruntime.Ptr, gpuNum)

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
		upload(context, datas[root-1], tmp)
		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		collective(mccl.BroadcastRing(gpuDriver, comms, root, datas, dataSize))

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			read(context, tmp, datas[i])
			for j := 0; j < dataSize; j++ {
				Expect(tmp[j]).To(Equal(float32(-0.0000123)))
			}
		}
	})

	It("AllReduce Test Buffer Size < Data Size", func() {
		gpuNum := 4
		var dataSize uint32 = 1029
		var bufSize uint32 = 256
		datas := make([]gpuruntime.Ptr, gpuNum)
		bufs := make([]gpuruntime.Ptr, gpuNum)
		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			for j := 0; j < int(dataSize); j++ {
				tmp[j] = float32(i + 1)
			}
			gpuDriver.SelectGPU(context, i+1)
			data := gpuDriver.AllocateMemory(context, uint64(dataSize*4))
			upload(context, data, tmp)
			buf := gpuDriver.AllocateMemory(context, uint64(bufSize*4))
			gpuIDs = append(gpuIDs, i+1)
			datas[i] = data
			bufs[i] = buf
		}

		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		collective(mccl.AllReduceRing(
			gpuDriver, comms, datas, int(dataSize), bufs, int(bufSize)))

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			read(context, tmp, datas[i])
			for j := 0; j < int(dataSize); j++ {
				Expect(tmp[j]).To(Equal(float32(2.5)))
			}
			log.Printf("Passed")
		}
	})

	It("AllReduce Test Buffer Size == Data Size", func() {
		gpuNum := 4
		var dataSize uint32 = 1029
		var bufSize uint32 = 1029
		datas := make([]gpuruntime.Ptr, gpuNum)
		bufs := make([]gpuruntime.Ptr, gpuNum)
		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			for j := 0; j < int(dataSize); j++ {
				tmp[j] = float32(i + 1)
			}
			gpuDriver.SelectGPU(context, i+1)
			data := gpuDriver.AllocateMemory(context, uint64(dataSize*4))
			upload(context, data, tmp)
			buf := gpuDriver.AllocateMemory(context, uint64(bufSize*4))
			gpuIDs = append(gpuIDs, i+1)
			datas[i] = data
			bufs[i] = buf
		}

		comms = mccl.CommInitAll(gpuNum, gpuDriver, context, gpuIDs)
		collective(mccl.AllReduceRing(
			gpuDriver, comms, datas, int(dataSize), bufs, int(bufSize)))

		for i := 0; i < gpuNum; i++ {
			tmp := make([]float32, uint64(dataSize))
			gpuDriver.SelectGPU(context, i+1)
			read(context, tmp, datas[i])
			for j := 0; j < int(dataSize); j++ {
				Expect(tmp[j]).To(Equal(float32(2.5)))
			}
			log.Printf("Passed")
		}
	})
})
