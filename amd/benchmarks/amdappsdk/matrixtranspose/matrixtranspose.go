// Package matrixtranspose implements the matrix transpose benchmark from
// AMDAPPSDK.
package matrixtranspose

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Output              driver.Ptr
	Input               driver.Ptr
	Block               driver.LocalPtr
	WIWidth             uint32
	WIHeight            uint32
	NumWGWidth          uint32
	GroupXOffset        uint32
	GroupYOffset        uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.HsaCo

	Width              int
	elemsPerThread1Dim int
	blockSize          int

	hInputData  []uint32
	hOutputData []uint32
	dInputData  driver.Ptr
	dOutputData driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	b.elemsPerThread1Dim = 4
	b.blockSize = 16
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "matrixTranspose")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Width

	b.hInputData = make([]uint32, numData)
	b.hOutputData = make([]uint32, numData)

	for i := 0; i < numData; i++ {
		b.hInputData[i] = uint32(i)
	}

	if b.useUnifiedMemory {
		b.dInputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(numData*4))
		b.dOutputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(numData*4))
	} else {
		b.dInputData = b.driver.AllocateMemory(
			b.context, uint64(numData*4))
		b.dOutputData = b.driver.AllocateMemory(
			b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dInputData, uint64(numData*4), b.gpus)
		b.driver.Distribute(b.context, b.dOutputData, uint64(numData*4), b.gpus)
	}

	b.driver.MemCopyH2D(b.context, b.dInputData, b.hInputData)
}

func (b *Benchmark) exec() {
	wiWidth := uint32(b.Width / b.elemsPerThread1Dim)
	wiHeight := uint32(b.Width / b.elemsPerThread1Dim)
	numWGWidth := wiWidth / uint32(b.blockSize)
	wgXPerGPU := numWGWidth / uint32(len(b.queues))

	for i, queue := range b.queues {
		wiWidthPerGPU := int(wiWidth) / len(b.queues)

		kernArg := KernelArgs{
			b.dOutputData,
			b.dInputData,
			driver.LocalPtr(b.blockSize * b.blockSize *
				b.elemsPerThread1Dim * b.elemsPerThread1Dim * 4),
			wiWidth, wiHeight, numWGWidth,
			wgXPerGPU * uint32(i), 0,
			0, 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{uint32(wiWidthPerGPU), wiHeight, 1},
			[3]uint16{uint16(b.blockSize), uint16(b.blockSize), 1},
			&kernArg,
		)
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputData, b.dOutputData)
}

// Verify verifies
func (b *Benchmark) Verify() {
	failed := false
	for i := 0; i < b.Width; i++ {
		for j := 0; j < b.Width; j++ {
			actual := b.hOutputData[i*b.Width+j]
			expected := b.hInputData[j*b.Width+i]
			if expected != actual {
				log.Printf("mismatch at (%d, %d), expected %d, but get %d\n",
					i, j, expected, actual)
				failed = true
			}
		}
	}

	if failed {
		panic("failed to verify matrix transpose result")
	}
	log.Printf("Passed!\n")
}
