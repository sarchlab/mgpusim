// Package vectoradd implements the VectorAdd benchmark from AMDAPPSDK.
package vectoradd

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	A                         driver.Ptr
	B                         driver.Ptr
	C                         driver.Ptr
	Width                     int32
	Height                    int32
	Padding                   int32
	OffsetX, OffsetY, OffsetZ uint64
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	kernel  *insts.KernelCodeObject
	gpus    []int

	Width  uint32
	Height uint32

	hA []float32
	hB []float32
	hC []float32

	dA driver.Ptr
	dB driver.Ptr
	dC driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "_Z15vectoradd_floatPfPKfS1_ii")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Height

	b.hA = make([]float32, numData)
	b.hB = make([]float32, numData)
	b.hC = make([]float32, numData)

	for i := uint32(0); i < numData; i++ {
		b.hB[i] = float32(i)
		b.hC[i] = float32(i) * 100.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dA, uint64(numData*4), b.gpus)
		b.dB = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dB, uint64(numData*4), b.gpus)
		b.dC = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dC, uint64(numData*4), b.gpus)
	}

	b.driver.MemCopyH2D(b.context, b.dB, b.hB)
	b.driver.MemCopyH2D(b.context, b.dC, b.hC)
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))
	numData := b.Width * b.Height

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)

		gridSize := numData / uint32(len(b.gpus))

		kernArg := KernelArgs{
			A:       b.dA,
			B:       b.dB,
			C:       b.dC,
			Width:   int32(b.Width),
			Height:  int32(b.Height),
			OffsetX: uint64(gridSize * uint32(i)),
		}

		b.driver.EnqueueLaunchKernel(
			queues[i],
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hA, b.dA)
}

// Verify verifies
func (b *Benchmark) Verify() {
	numData := b.Width * b.Height
	mismatch := false

	for i := uint32(0); i < numData; i++ {
		expected := b.hB[i] + b.hC[i]
		if b.hA[i] != expected {
			log.Printf("mismatch at position %d. Expected %f, but got %f",
				i, expected, b.hA[i])
			mismatch = true
		}
	}

	if mismatch {
		panic("verification failed")
	}

	log.Printf("Passed!\n")
}
