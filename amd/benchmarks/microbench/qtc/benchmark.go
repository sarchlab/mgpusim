// Package qtc implements the SHOC QTC (Quality Threshold Clustering) benchmark.
// It computes pairwise distances between 2D points and counts neighbors
// within a threshold distance for each point.
package qtc

import (
	"log"
	"math"
	"math/rand"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for qtc_kernel.
type KernelArgs struct {
	X         driver.Ptr
	Y         driver.Ptr
	Counts    driver.Ptr
	N         int32
	Threshold float32
}

// Benchmark defines the QTC benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hX      []float32
	hY      []float32
	hCounts []int32

	dX      driver.Ptr
	dY      driver.Ptr
	dCounts driver.Ptr

	threshold float32
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new QTC benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1024
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

// SetNumElements sets the number of points.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "qtc_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load qtc_kernel")
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
	rng := rand.New(rand.NewSource(42))

	// Threshold: sqrt(N)/10
	b.threshold = float32(math.Sqrt(float64(n)) / 10.0)

	// Initialize random 2D points
	b.hX = make([]float32, n)
	b.hY = make([]float32, n)
	b.hCounts = make([]int32, n)

	for i := 0; i < n; i++ {
		b.hX[i] = rng.Float32() * float32(math.Sqrt(float64(n)))
		b.hY[i] = rng.Float32() * float32(math.Sqrt(float64(n)))
	}

	// Allocate GPU memory
	floatBytes := uint64(n * 4)
	intBytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dX = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dY = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dCounts = b.driver.AllocateUnifiedMemory(b.context, intBytes)
	} else {
		b.dX = b.driver.AllocateMemory(b.context, floatBytes)
		b.dY = b.driver.AllocateMemory(b.context, floatBytes)
		b.dCounts = b.driver.AllocateMemory(b.context, intBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dX, b.hX)
	b.driver.MemCopyH2D(b.context, b.dY, b.hY)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		X:         b.dX,
		Y:         b.dY,
		Counts:    b.dCounts,
		N:         int32(n),
		Threshold: b.threshold,
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hCounts, b.dCounts)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	thresh2 := float64(b.threshold) * float64(b.threshold)
	errors := 0

	for i := 0; i < n; i++ {
		xi := float64(b.hX[i])
		yi := float64(b.hY[i])
		expected := 0

		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			dx := xi - float64(b.hX[j])
			dy := yi - float64(b.hY[j])
			dist2 := dx*dx + dy*dy
			if dist2 <= thresh2 {
				expected++
			}
		}

		got := int(b.hCounts[i])
		if got != expected {
			if errors < 10 {
				log.Printf("Mismatch at point %d: got %d, expected %d",
					i, got, expected)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in qtc verification", errors)
	}
	log.Printf("Passed!")
}
