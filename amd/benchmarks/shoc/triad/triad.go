// Package triad implements the SHOC Triad (stream triad) benchmark.
// Computes a[i] = b[i] + scalar * c[i] — a classic memory bandwidth benchmark.
package triad

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	C                   driver.Ptr
	Scalar              float32
	N                   int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines the triad benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	triadKernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int
	Scalar      float32

	useUnifiedMemory bool

	hB []float32
	hC []float32
	hA []float32 // GPU result copied back

	dA driver.Ptr
	dB driver.Ptr
	dC driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new triad benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.BlockSize = 256
	b.Scalar = 1.75
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

// SetNumElements sets the number of elements
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

func (b *Benchmark) loadProgram() {
	b.triadKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "triad_kernel")
	if b.triadKernel == nil {
		log.Panic("Failed to load triad_kernel binary")
	}
}

// Run runs the benchmark
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

	// Initialize with known pattern matching HIP source
	b.hB = make([]float32, n)
	b.hC = make([]float32, n)
	b.hA = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hB[i] = float32(i%1000) * 0.001
		b.hC[i] = float32((i+37)%1000) * 0.001
	}

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dC = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytes)
		b.dB = b.driver.AllocateMemory(b.context, bytes)
		b.dC = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dB, b.hB)
	b.driver.MemCopyH2D(b.context, b.dC, b.hC)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		A:                 b.dA,
		B:                 b.dB,
		C:                 b.dC,
		Scalar:            b.Scalar,
		N:                 int32(n),
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  localSize[1],
		HiddenGroupSizeZ:  localSize[2],
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context,
		b.triadKernel,
		globalSize, localSize,
		&args,
	)

	// Copy result back
	b.driver.MemCopyD2H(b.context, b.hA, b.dA)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	for i := 0; i < n; i++ {
		expected := b.hB[i] + b.Scalar*b.hC[i]
		diff := math.Abs(float64(b.hA[i] - expected))
		tol := 1e-5*math.Abs(float64(expected)) + 1e-6

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hA[i], expected)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in triad verification", errors)
	}
	log.Printf("Passed!")
}
