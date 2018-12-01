package fir

import (
	"log"
	"math"

	"gitlab.com/akita/gcn3/kernels"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
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
	driver *driver.Driver
	hsaco  *insts.HsaCo

	numGPUs   int
	gpuQueues []*driver.CommandQueue

	Length       int
	numTaps      int
	outputData   []float32
	inputData    []float32
	filterData   []float32
	gFilterData  []driver.GPUPtr
	gHistoryData []driver.GPUPtr
	gInputData   driver.GPUPtr
	gOutputData  driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.numGPUs = driver.GetNumGPUs()
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gpuQueues = append(b.gpuQueues, b.driver.CreateCommandQueue())
	}

	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "FIR")

	return b
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.numTaps = 16

	b.gInputData = b.driver.AllocateMemory(uint64(b.Length * 4))
	b.gOutputData = b.driver.AllocateMemory(uint64(b.Length * 4))
	bytePerGPU := uint64(b.Length / b.numGPUs * 4)

	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gFilterData = append(b.gFilterData,
			b.driver.AllocateMemory(uint64(b.numTaps*4)))
		b.gHistoryData = append(b.gHistoryData,
			b.driver.AllocateMemory(uint64(b.numTaps*4)))

		b.driver.Remap(uint64(b.gInputData)+uint64(i)*bytePerGPU, bytePerGPU, i)
		b.driver.Remap(uint64(b.gOutputData)+uint64(i)*bytePerGPU, bytePerGPU, i)
	}

	b.filterData = make([]float32, b.numTaps)
	for i := 0; i < b.numTaps; i++ {
		b.filterData[i] = float32(i)
	}

	b.inputData = make([]float32, b.Length)
	b.outputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = float32(i)
	}

	for i := 0; i < b.numGPUs; i++ {
		b.driver.EnqueueMemCopyH2D(
			b.gpuQueues[i], b.gFilterData[i], b.filterData)
		b.driver.EnqueueMemCopyH2D(
			b.gpuQueues[i],
			driver.GPUPtr(uint64(b.gInputData)+uint64(i)*bytePerGPU),
			b.inputData[b.Length/b.numGPUs*i:b.Length/b.numGPUs*(i+1)])
	}
}

func (b *Benchmark) exec() {
	bytePerGPU := uint64(b.Length / b.numGPUs * 4)
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		kernArg := KernelArgs{
			//driver.GPUPtr(uint64(b.gOutputData) + uint64(i)*bytePerGPU),
			b.gOutputData,
			b.gFilterData[i],
			//driver.GPUPtr(uint64(b.gInputData) + uint64(i)*bytePerGPU),
			b.gInputData,
			b.gHistoryData[i],
			uint32(b.numTaps),
			0,
			int64(i * b.Length / b.numGPUs), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			b.gpuQueues[i],
			b.hsaco,
			[3]uint32{uint32(b.Length / b.numGPUs), 1, 1},
			[3]uint16{256, 1, 1},
			&kernArg,
		)

	}

	b.driver.ExecuteAllCommands()

	for i := 0; i < b.numGPUs; i++ {
		b.driver.EnqueueMemCopyD2H(
			b.gpuQueues[i],
			b.outputData[b.Length/b.numGPUs*i:b.Length/b.numGPUs*(i+1)],
			driver.GPUPtr(uint64(b.gOutputData)+uint64(i)*bytePerGPU))
	}

	b.driver.ExecuteAllCommands()
}

func (b *Benchmark) Verify() {
	for i := 0; i < b.Length; i++ {
		var sum float32
		sum = 0

		for j := 0; j < b.numTaps; j++ {
			if i < j {
				continue
			}
			sum += b.inputData[i-j] * b.filterData[j]
		}

		if math.Abs(float64(sum-b.outputData[i])) >= 1e-5 {
			log.Fatalf("At position %d, expected %f, but get %f.\n",
				i, sum, b.outputData[i])
		}
	}

	log.Printf("Passed!\n")
}
