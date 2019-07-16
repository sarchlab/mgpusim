package pagerank

import (
	"fmt"
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type PageRankKernelArgs struct {
	NumRows   uint32
	RowOffset driver.GPUPtr
	Col       driver.GPUPtr
	Val       driver.GPUPtr
	Vals      driver.LocalPtr
	X         driver.GPUPtr
	Y         driver.GPUPtr
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.HsaCo

	NumNodes       uint32
	NumConnections uint32
	MaxIterations  uint32

	hPageRank      []float32
	hRowOffsets    []uint32
	hColumnNumbers []uint32
	hValues        []uint32

	dPageRank      driver.GPUPtr
	dPageRankTemp  driver.GPUPtr
	dRowOffsets    driver.GPUPtr
	dColumnNumbers driver.GPUPtr
	dValues        driver.GPUPtr
	dLocalValues   driver.LocalPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := FSMustByte(false, "/kernels.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "PageRankUpdateGpu")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {

	initData := float32(1.0) / float32(b.NumNodes)
	b.hPageRank = make([]float32, b.NumNodes)
	b.hRowOffsets = make([]uint32, b.NumNodes+1)
	b.hColumnNumbers = make([]uint32, b.NumConnections)
	b.hValues = make([]uint32, b.NumConnections)

	for i := uint32(0); i < b.NumNodes; i++ {
		b.hPageRank[i] = initData
	}

	b.dPageRank = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dPageRankTemp = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dRowOffsets = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dColumnNumbers = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dValues = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)

	b.driver.MemCopyH2D(b.context, b.dPageRank, b.hPageRank)
	b.driver.MemCopyH2D(b.context, b.dPageRankTemp, b.hPageRank)
}

func (b *Benchmark) exec() {

	//b.dLocalValues = driver.LocalPtr(driver.AllocateMemoryWithAlignment(
	//	b.context, uint64(64) * 4, 4096) )

	//temp = make([]float32, 64)
	//b.dLocalValues = driver.LocalPtr(temp)

	localWorkSize := 8
	i := uint32(0)

	for _, queue := range b.queues {

		for i = 0; i < b.MaxIterations; i++ {

			kernArg := PageRankKernelArgs{
				b.NumNodes,
				b.dRowOffsets,
				b.dColumnNumbers,
				b.dValues,
				b.dLocalValues,
				b.dPageRank,
				b.dPageRankTemp,
			}

			if i%2 == 0 {
				kernArg = PageRankKernelArgs{
					b.NumNodes,
					b.dRowOffsets,
					b.dColumnNumbers,
					b.dValues,
					b.dLocalValues,
					b.dPageRank,
					b.dPageRankTemp,
				}
			} else {
				kernArg = PageRankKernelArgs{
					b.NumNodes,
					b.dRowOffsets,
					b.dColumnNumbers,
					b.dValues,
					b.dLocalValues,
					b.dPageRankTemp,
					b.dPageRank,
				}
			}

			b.driver.EnqueueLaunchKernel(
				queue,
				b.kernel,
				[3]uint32{uint32(b.NumNodes), uint32(64), 1},
				[3]uint16{uint16(localWorkSize), uint16(localWorkSize), 1},
				&kernArg,
			)
		}
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	if i%2 != 0 {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRank)
	} else {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRankTemp)
	}

	for i := 0; i < len(b.hPageRank); i++ {
		fmt.Printf("%d: %d", i, b.hPageRank[i])
	}
}

func (b *Benchmark) Verify() {
	panic("Verify Function not implemented yet")
}
