// Package busspeedreadback implements bus speed readback (D2H) microbenchmark.
package busspeedreadback

import (
	"log"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for bus_speed_readback_kernel.
type KernelArgs struct {
	A    driver.Ptr
	B    driver.Ptr
	N    int32
	Pad0 int32
}

// Benchmark defines the bus speed readback benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hA []float32
	hB []float32

	dA driver.Ptr
	dB driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new bus speed readback benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetNumElements sets the number of elements.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "bus_speed_readback_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load bus_speed_readback_kernel")
	}
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	n := b.NumElements

	b.hA = make([]float32, n)
	b.hB = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hA[i] = float32(i%1000) * 0.001
	}

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytes)
		b.dB = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		A: b.dA,
		B: b.dB,
		N: int32(n),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hB, b.dB)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	for i := 0; i < n; i++ {
		if b.hB[i] != b.hA[i] {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hB[i], b.hA[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in bus_speed_readback verification",
			errors)
	}
	log.Printf("Passed!")
}
