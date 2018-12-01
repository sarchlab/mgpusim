package bitonicsort

import (
	"log"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type BitonicKernelArgs struct {
	Input               driver.GPUPtr
	Stage               uint32
	PassOfStage         uint32
	Direction           uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver *driver.Driver
	hsaco  *insts.HsaCo

	numGPUs   int
	gpuQueues []*driver.CommandQueue

	Length         int
	OrderAscending bool

	inputData  []uint32
	outputData []uint32
	gInputData driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver

	b.numGPUs = driver.GetNumGPUs()
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gpuQueues = append(b.gpuQueues, b.driver.CreateCommandQueue())
	}

	b.loadProgram()
	return b
}

func (b *Benchmark) loadProgram() {
	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}

	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "BitonicSort")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {

	b.inputData = make([]uint32, b.Length)
	b.outputData = make([]uint32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = rand.Uint32()
	}

	bytePerGPU := uint64(b.Length / b.numGPUs * 4)
	b.gInputData = b.driver.AllocateMemory(uint64(b.Length * 4))
	for i := 0; i < b.numGPUs; i++ {
		addr := uint64(b.gInputData) + uint64(i)*bytePerGPU
		b.driver.Remap(addr, bytePerGPU, i)

		b.driver.EnqueueMemCopyH2D(
			b.gpuQueues[i], driver.GPUPtr(addr),
			b.inputData[b.Length/b.numGPUs*i:b.Length/b.numGPUs*(i+1)])
	}
	b.driver.ExecuteAllCommands()

}

func (b *Benchmark) exec() {

	numStages := 0
	for temp := b.Length; temp > 1; temp >>= 1 {
		numStages++
	}

	direction := 1
	if b.OrderAscending == false {
		direction = 0
	}

	for stage := 0; stage < numStages; stage += 1 {
		for passOfStage := 0; passOfStage < stage+1; passOfStage++ {
			for i := 0; i < b.numGPUs; i++ {
				kernArg := BitonicKernelArgs{
					b.gInputData,
					uint32(stage),
					uint32(passOfStage),
					uint32(direction),
					int64(b.Length / b.numGPUs / 2), 0, 0}
				b.driver.EnqueueLaunchKernel(
					b.gpuQueues[i],
					b.hsaco,
					[3]uint32{uint32(b.Length / 2), 1, 1},
					[3]uint16{256, 1, 1},
					&kernArg)
			}
			b.driver.ExecuteAllCommands()
		}
	}

	bytePerGPU := uint64(b.Length / b.numGPUs * 4)
	for i := 0; i < b.numGPUs; i++ {
		b.driver.EnqueueMemCopyD2H(
			b.gpuQueues[i],
			b.outputData[b.Length/b.numGPUs*i:b.Length/b.numGPUs*(i+1)],
			driver.GPUPtr(uint64(b.gInputData)+uint64(i)*bytePerGPU))
	}
}

func (b *Benchmark) Verify() {

	for i := 0; i < b.Length-1; i++ {
		if b.OrderAscending {
			if b.outputData[i] > b.outputData[i+1] {
				log.Panicf("Error: array[%d] > array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		} else {
			if b.outputData[i] < b.outputData[i+1] {
				log.Panicf("Error: array[%d] < array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		}
	}

	log.Printf("Passed!\n")
}
