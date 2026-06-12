// Package sharedbw implements shared memory (LDS) bandwidth microbenchmark.
package sharedbw

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for shared_bw_kernel.
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	N                   int32
	Pad0                int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad1                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines the shared memory bandwidth benchmark.
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

// NewBenchmark creates a new shared memory bandwidth benchmark.
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
		hsacoBytes, "shared_bw_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load shared_bw_kernel")
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
		b.hA[i] = float32(i%1000)*0.001 + 1.0
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
		A:                 b.dA,
		B:                 b.dB,
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
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hB, b.dB)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	for i := 0; i < n; i++ {
		expected := float64(b.hA[i])
		for j := 0; j < 128; j++ {
			expected = expected*1.00001 + 0.00001
		}
		diff := math.Abs(float64(b.hB[i]) - expected)
		tol := 1e-2*math.Abs(expected) + 1e-5

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hB[i], expected)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in shared_bw verification", errors)
	}
	log.Printf("Passed!")
}
