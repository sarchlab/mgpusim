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
	gpusToUse []int
	driver    *driver.Driver
	context   *driver.Context

	hsaco *insts.HsaCo

	Length         int
	OrderAscending bool

	inputData  []uint32
	gInputData driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.gpusToUse = []int{1}
	b.driver = driver
	b.context = driver.Init()
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

func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpusToUse = gpuIDs
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.inputData = make([]uint32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = rand.Uint32()
	}

	b.gInputData = b.driver.AllocateMemory(b.context, uint64(b.Length*4))
	b.driver.Distribute(
		b.context,
		uint64(b.gInputData), uint64(b.Length*4),
		b.gpusToUse)

	b.driver.MemCopyH2D(b.context, b.gInputData, b.inputData)
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

	queues := []*driver.CommandQueue{}
	for _, gpuID := range b.gpusToUse {
		b.driver.SelectGPU(b.context, gpuID)
		queues = append(queues, b.driver.CreateCommandQueue(b.context))
	}

	for stage := 0; stage < numStages; stage++ {
		for passOfStage := 0; passOfStage < stage+1; passOfStage++ {
			totalWIs := b.Length / 2
			wiPerQueue := totalWIs / len(queues)
			remainder := totalWIs % len(queues)

			for i, q := range queues {
				kernArg := BitonicKernelArgs{
					b.gInputData,
					uint32(stage),
					uint32(passOfStage),
					uint32(direction),
					int64(wiPerQueue * i), 0, 0}

				numWi := wiPerQueue
				if i == len(queues)-1 {
					numWi += remainder
				}

				b.driver.EnqueueLaunchKernel(
					q,
					b.hsaco,
					[3]uint32{uint32(numWi), 1, 1},
					[3]uint16{256, 1, 1},
					&kernArg,
				)
			}

			for _, q := range queues {
				b.driver.DrainCommandQueue(q)
			}
		}
	}
}

func (b *Benchmark) Verify() {
	gpuOutput := make([]uint32, b.Length)
	b.driver.MemCopyD2H(b.context, gpuOutput, b.gInputData)

	for i := 0; i < b.Length-1; i++ {
		if b.OrderAscending {
			if gpuOutput[i] > gpuOutput[i+1] {
				log.Fatalf("Error: array[%d] > array[%d]: %d %d\n", i, i+1,
					gpuOutput[i], gpuOutput[i+1])
			}
		} else {
			if gpuOutput[i] < gpuOutput[i+1] {
				log.Fatalf("Error: array[%d] < array[%d]: %d %d\n", i, i+1,
					gpuOutput[i], gpuOutput[i+1])
			}
		}
	}

	log.Printf("Passed!\n")
}
