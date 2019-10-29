package fir

import (
	"log"
	"math"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type KernelArgs struct {
	Output              driver.GPUPtr
	Filter              driver.GPUPtr
	Input               driver.GPUPtr
	History             driver.GPUPtr
	NumTaps             uint32
	Padding             uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.HsaCo
	gpus    []int

	Length       int
	numTaps      int
	inputData    []float32
	filterData   []float32
	gFilterData  []driver.GPUPtr
	gHistoryData driver.GPUPtr
	gInputData   driver.GPUPtr
	gOutputData  driver.GPUPtr

	useUnifiedMemory bool
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "FIR")

	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.numTaps = 16

	b.filterData = make([]float32, b.numTaps)
	for i := 0; i < b.numTaps; i++ {
		b.filterData[i] = float32(i)
	}

	b.inputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = float32(i)
	}

	if b.useUnifiedMemory {
		b.gFilterData = make([]driver.GPUPtr, len(b.gpus))
		b.gHistoryData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.numTaps*4))
		b.gInputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.Length*4))
		b.gOutputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.Length*4))
	} else {
		b.gFilterData = make([]driver.GPUPtr, len(b.gpus))
		b.gHistoryData = b.driver.AllocateMemory(
			b.context, uint64(b.numTaps*4))
		b.gInputData = b.driver.AllocateMemory(
			b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context,
			b.gInputData, uint64(b.Length*4), b.gpus)
		b.gOutputData = b.driver.AllocateMemory(
			b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context,
			b.gOutputData, uint64(b.Length*4), b.gpus)
	}

	b.driver.MemCopyH2D(b.context, b.gInputData, b.inputData)

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		if b.useUnifiedMemory {
			b.gFilterData[i] = b.driver.AllocateUnifiedMemory(
				b.context, uint64(b.numTaps*4))
		} else {
			b.gFilterData[i] = b.driver.AllocateMemory(
				b.context, uint64(b.numTaps*4))
		}
		b.driver.MemCopyH2D(b.context, b.gFilterData[i], b.filterData)
	}
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))
	numWi := b.Length

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)

		kernArg := KernelArgs{
			b.gOutputData,
			b.gFilterData[i],
			b.gInputData,
			b.gHistoryData,
			uint32(b.numTaps),
			0,
			int64(i * numWi / len(b.gpus)), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queues[i],
			b.hsaco,
			[3]uint32{uint32(numWi / len(b.gpus)), 1, 1},
			[3]uint16{256, 1, 1}, &kernArg,
		)
	}

	for i := range b.gpus {
		b.driver.DrainCommandQueue(queues[i])
	}
}

func (b *Benchmark) Verify() {
	gpuOutput := make([]float32, b.Length)
	b.driver.MemCopyD2H(b.context, gpuOutput, b.gOutputData)

	for i := 0; i < b.Length; i++ {
		var sum float32
		sum = 0

		for j := 0; j < b.numTaps; j++ {
			if i < j {
				continue
			}
			sum += b.inputData[i-j] * b.filterData[j]
		}

		if math.Abs(float64(sum-gpuOutput[i])) >= 1e-5 {
			log.Fatalf("At position %d, expected %f, but get %f.\n",
				i, sum, gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")
}
