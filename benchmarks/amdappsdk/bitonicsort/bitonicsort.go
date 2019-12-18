package bitonicsort

import (
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

var doPerPassVerify = true

type BitonicKernelArgs struct {
	Input               driver.GPUPtr
	Stage               uint32
	PassOfStage         uint32
	Direction           uint32
	Padding             uint32
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

	inputData             []uint32
	outputData            []uint32
	gInputData            driver.GPUPtr
	perPassIn, perPassOut []uint32

	useUnifiedMemory bool
}

// NewBenchmark creates a new bitonic sort benchmark.
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

// SelectGPU selects the GPUs to use.
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpusToUse = gpuIDs
}

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark on simulated GPU platform
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpusToUse[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.inputData = make([]uint32, b.Length)
	b.outputData = make([]uint32, b.Length)
	for i := 0; i < b.Length; i++ {
		// b.inputData[i] = rand.Uint32()
		b.inputData[i] = uint32(i)
	}

	if doPerPassVerify {
		b.perPassIn = make([]uint32, b.Length)
		b.perPassOut = make([]uint32, b.Length)
	}

	if b.useUnifiedMemory {
		b.gInputData = b.driver.AllocateUnifiedMemory(b.context, uint64(b.Length*4))
	} else {
		b.gInputData = b.driver.AllocateMemory(b.context, uint64(b.Length*4))
		b.driver.Distribute(
			b.context,
			b.gInputData, uint64(b.Length*4),
			b.gpusToUse)
	}

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
			b.runPass(stage, passOfStage, direction, queues)
		}
	}

	b.driver.MemCopyD2H(b.context, b.outputData, b.gInputData)
}

func (b *Benchmark) runPass(
	stage, passOfStage int,
	direction int,
	queues []*driver.CommandQueue,
) {
	totalWIs := b.Length / 2
	wiPerQueue := totalWIs / len(queues)
	remainder := totalWIs % len(queues)

	log.Printf("Stage %d, pass %d\n", stage, passOfStage)

	if doPerPassVerify {
		b.driver.MemCopyD2H(b.context, b.perPassIn, b.gInputData)
	}

	for i, q := range queues {
		kernArg := BitonicKernelArgs{
			b.gInputData,
			uint32(stage),
			uint32(passOfStage),
			uint32(direction),
			0,
			int64(wiPerQueue * i), 0, 0,
		}

		numWi := wiPerQueue
		if i == len(queues)-1 {
			numWi += remainder
		}

		b.driver.EnqueueLaunchKernel(
			q,
			b.hsaco,
			[3]uint32{uint32(numWi), 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}

	if doPerPassVerify {
		b.driver.MemCopyD2H(b.context, b.perPassOut, b.gInputData)
		b.verifyPass(b.perPassIn, b.perPassOut, stage, passOfStage)
	}
}

func (b *Benchmark) verifyPass(in, out []uint32, stage, pass int) {
	cpuOut := make([]uint32, b.Length)

	pairDistance := 1 << uint32(stage-pass)
	blockWidth := 2 * pairDistance

	for i := 0; i < b.Length/2; i++ {
		leftID := (i % pairDistance) + (i/pairDistance)*blockWidth
		rightID := leftID + pairDistance

		sortIncreasing := uint32(0)
		if b.OrderAscending {
			sortIncreasing = 1
		}

		leftElement := in[leftID]
		rightElement := in[rightID]

		sameDirectionBlockWidth := 1 << uint32(stage)
		if ((i / sameDirectionBlockWidth) % 2) == 1 {
			sortIncreasing = 1 - sortIncreasing
		}

		greater := leftElement
		lesser := rightElement
		if leftElement < rightElement {
			greater, lesser = lesser, greater
		}

		if sortIncreasing == 1 {
			cpuOut[leftID] = lesser
			cpuOut[rightID] = greater
		} else {
			cpuOut[leftID] = greater
			cpuOut[rightID] = lesser
		}
	}

	// failed := false
	for i := 0; i < b.Length; i++ {
		if cpuOut[i] != out[i] {
			log.Panicf("Mismatch after stage %d pass %d at pos %d, expected %d, but get %d", stage, pass, i, cpuOut[i], out[i])
			// failed = true
		}
	}
	// if failed {
	// 	panic("failed")
	// }
}

// Verify checks if the array is sorted
func (b *Benchmark) Verify() {
	//for i := 0; i < b.Length; i++ {
	//	fmt.Printf("[%d]: %d\n", i, b.outputData[i])
	//}

	for i := 0; i < b.Length-1; i++ {
		if b.OrderAscending {
			if b.outputData[i] > b.outputData[i+1] {
				log.Fatalf("Error: array[%d] > array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		} else {
			if b.outputData[i] < b.outputData[i+1] {
				log.Fatalf("Error: array[%d] < array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		}
	}

	log.Printf("Passed!\n")
}
