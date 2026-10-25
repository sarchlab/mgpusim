// Package bitonicsort implements the bitonicsort benchmark from
// AMDAPPSDK.
package bitonicsort

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"log"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// GCN3 Kernel Arguments

// BitonicKernelArgs defines kernel arguments for GCN3
type BitonicKernelArgs struct {
	Input               driver.Ptr
	Stage               uint32
	PassOfStage         uint32
	Direction           uint32
	Padding             uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3 Kernel Arguments

// CDNA3BitonicKernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3BitonicKernelArgs struct {
	Input               driver.Ptr
	Stage               uint32
	PassOfStage         uint32
	Direction           uint32
	Padding             uint32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding2            [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	benchmarks.Progress
	initialConfig          checkpointConfig
	configCaptured         bool
	queues                 []driver.CommandQueue
	stage, pass, numStages int
	readback               driver.CommandID
	PerPassVerify          bool
	gpusToUse              []int
	driver                 *driver.Runtime
	context                driver.Context

	hsaco *insts.KernelCodeObject

	Arch           arch.Type
	Length         int
	OrderAscending bool

	inputData             []uint32
	outputData            []uint32
	gInputData            driver.Ptr
	perPassIn, perPassOut []uint32

	useUnifiedMemory bool
}

// NewBenchmark creates a new bitonic sort benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.gpusToUse = []int{1}
	b.driver = driver
	b.context = driver.Init()
	return b
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.hsaco = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "BitonicSort")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to use.
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpusToUse = gpuIDs
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.loadProgram()

	for _, gpu := range b.gpusToUse {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}
	b.driver.SelectGPU(b.context, b.gpusToUse[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	rng := rand.New(rand.NewSource(1))
	b.inputData = make([]uint32, b.Length)
	b.outputData = make([]uint32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = rng.Uint32()
		// b.inputData[i] = uint32(i)
	}

	if b.PerPassVerify && b.Verification {
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

	b.driver.EnqueueMemCopyH2D(b.queues[0], b.gInputData, b.inputData)
}

func (b *Benchmark) createCDNA3KernelArgs(stage, passOfStage, direction, numWi, offset int) CDNA3BitonicKernelArgs {
	return CDNA3BitonicKernelArgs{
		Input:               b.gInputData,
		Stage:               uint32(stage),
		PassOfStage:         uint32(passOfStage),
		Direction:           uint32(direction),
		HiddenBlockCountX:   uint32(numWi) / 64,
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    64,
		HiddenGroupSizeY:    1,
		HiddenGroupSizeZ:    1,
		HiddenRemainderX:    uint16(uint32(numWi) % 64),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: int64(offset),
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
}

func (b *Benchmark) createGCN3KernelArgs(stage, passOfStage, direction, offset int) BitonicKernelArgs {
	return BitonicKernelArgs{
		b.gInputData,
		uint32(stage),
		uint32(passOfStage),
		uint32(direction),
		0,
		int64(offset), 0, 0,
	}
}

func (b *Benchmark) runPass(
	stage, passOfStage int,
	direction int,
	queues []driver.CommandQueue,
) {
	totalWIs := b.Length / 2
	wiPerQueue := totalWIs / len(queues)
	remainder := totalWIs % len(queues)

	fmt.Fprintf(os.Stderr, "Stage %d, pass %d\n", stage, passOfStage)

	for i, q := range queues {
		numWi := wiPerQueue
		if i == len(queues)-1 {
			numWi += remainder
		}

		if b.Arch == arch.CDNA3 {
			kernArg := b.createCDNA3KernelArgs(stage, passOfStage, direction, numWi, wiPerQueue*i)
			b.driver.EnqueueLaunchKernel(
				q,
				b.hsaco,
				[3]uint32{uint32(numWi), 1, 1},
				[3]uint16{64, 1, 1},
				&kernArg,
			)
		} else {
			kernArg := b.createGCN3KernelArgs(stage, passOfStage, direction, wiPerQueue*i)
			b.driver.EnqueueLaunchKernel(
				q,
				b.hsaco,
				[3]uint32{uint32(numWi), 1, 1},
				[3]uint16{64, 1, 1},
				&kernArg,
			)
		}
	}
}

func (b *Benchmark) verifyPass(in, out []uint32, stage, pass int) error {
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
			return fmt.Errorf(
				"Mismatch after stage %d pass %d at pos %d, expected %d, but get %d",
				stage,
				pass,
				i,
				cpuOut[i],
				out[i],
			)
			// failed = true
		}
	}
	// if failed {
	// 	return fmt.Errorf("%s",fmt.Sprint("failed"))
	// }

	return nil
}

// Verify checks if the array is sorted
func (b *Benchmark) verify() error {
	// for i := 0; i < b.Length; i++ {
	// 	fmt.Printf("[%d]: %d\n", i, b.outputData[i])
	// }

	for i := 0; i < b.Length-1; i++ {
		if b.OrderAscending {
			if b.outputData[i] > b.outputData[i+1] {
				return fmt.Errorf("Error: array[%d] > array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		} else {
			if b.outputData[i] < b.outputData[i+1] {
				return fmt.Errorf("Error: array[%d] < array[%d]: %d %d\n", i, i+1,
					b.outputData[i], b.outputData[i+1])
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.begin()
	case 1:
		if b.stage == b.numStages {
			b.Phase = 5
			return nil
		}
		if b.PerPassVerify && b.Verification {
			b.readback = b.driver.EnqueueMemCopyD2H(b.queues[0], b.gInputData, b.Length*4)
			b.Phase = 2
		} else {
			b.submitPass()
		}
	case 2:
		if err := b.driver.ReadResult(b.readback, b.perPassIn); err != nil {
			return err
		}
		b.submitPass()
	case 3:
		if b.PerPassVerify && b.Verification {
			b.readback = b.driver.EnqueueMemCopyD2H(b.queues[0], b.gInputData, b.Length*4)
			b.Phase = 4
		} else {
			b.nextPass()
		}
	case 4:
		if err := b.driver.ReadResult(b.readback, b.perPassOut); err != nil {
			return err
		}
		if err := b.verifyPass(b.perPassIn, b.perPassOut, b.stage, b.pass); err != nil {
			return err
		}
		b.nextPass()
	case 5:
		b.readback = b.driver.EnqueueMemCopyD2H(b.queues[0], b.gInputData, b.Length*4)
		b.Phase = 6
	case 6:
		if err := b.driver.ReadResult(b.readback, b.outputData); err != nil {
			return err
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}
func (b *Benchmark) submitPass() {
	direction := 0
	if b.OrderAscending {
		direction = 1
	}
	b.runPass(b.stage, b.pass, direction, b.queues)
	b.Phase = 3
}
func (b *Benchmark) nextPass() {
	b.pass++
	if b.pass > b.stage {
		b.stage++
		b.pass = 0
	}
	b.Phase = 1
}

func (b *Benchmark) begin() {
	b.initialConfig = b.checkpointConfig()
	b.configCaptured = true
	b.setup()
	for n := b.Length; n > 1; n >>= 1 {
		b.numStages++
	}
	b.Phase = 1
}
