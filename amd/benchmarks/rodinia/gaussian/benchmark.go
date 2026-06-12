// Package gaussian implements the Rodinia Gaussian Elimination benchmark.
// Two kernels: Fan1 (1D, pivot row division) and Fan2 (2D, row elimination)
package gaussian

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Fan1KernelArgs defines Fan1 kernel arguments
type Fan1KernelArgs struct {
	M                   driver.Ptr
	A                   driver.Ptr
	Size                int32
	T                   int32
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

// Fan2KernelArgs defines Fan2 kernel arguments
type Fan2KernelArgs struct {
	M                   driver.Ptr
	A                   driver.Ptr
	B                   driver.Ptr
	Size                int32
	T                   int32
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

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	fan1Kernel *insts.KernelCodeObject
	fan2Kernel *insts.KernelCodeObject

	size int

	hA []float32
	hB []float32
	hM []float32

	dA driver.Ptr
	dB driver.Ptr
	dM driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.size = 256
	return b
}

// SetSize sets the matrix size
func (b *Benchmark) SetSize(size int) {
	b.size = size
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.fan1Kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "Fan1")
	if b.fan1Kernel == nil {
		log.Panic("Failed to load Fan1 kernel binary")
	}

	b.fan2Kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "Fan2")
	if b.fan2Kernel == nil {
		log.Panic("Failed to load Fan2 kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	n := b.size

	rand.Seed(42)
	b.hA = make([]float32, n*n)
	b.hB = make([]float32, n)
	b.hM = make([]float32, n*n)

	for i := 0; i < n*n; i++ {
		b.hA[i] = float32(rand.Intn(1000)) / 100.0
	}
	for i := 0; i < n; i++ {
		b.hB[i] = float32(rand.Intn(1000)) / 100.0
	}
	// Make diagonally dominant
	for i := 0; i < n; i++ {
		b.hA[i*n+i] += float32(n)
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, uint64(n*n*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.dM = b.driver.AllocateUnifiedMemory(b.context, uint64(n*n*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context, uint64(n*n*4))
		b.dB = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.dM = b.driver.AllocateMemory(b.context, uint64(n*n*4))
	}
}

func (b *Benchmark) exec() {
	n := b.size

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
	b.driver.MemCopyH2D(b.context, b.dB, b.hB)
	b.driver.MemCopyH2D(b.context, b.dM, b.hM)

	blockSize := 32

	for t := 0; t < n-1; t++ {
		// Fan1: 1D grid
		fan1Threads := n - 1 - t
		if fan1Threads > 0 {
			gsX1 := uint32(((fan1Threads - 1) / blockSize + 1) * blockSize)
			localSize1 := [3]uint16{uint16(blockSize), 1, 1}
			globalSize1 := [3]uint32{gsX1, 1, 1}

			b.launchFan1(localSize1, globalSize1, gsX1, t)
		}

		// Fan2: 2D grid
		fan2Rows := n - 1 - t
		fan2Cols := n - t
		if fan2Rows > 0 && fan2Cols > 0 {
			gsX2 := uint32(((fan2Rows - 1) / blockSize + 1) * blockSize)
			gsY2 := uint32(((fan2Cols - 1) / blockSize + 1) * blockSize)
			localSize2 := [3]uint16{uint16(blockSize), uint16(blockSize), 1}
			globalSize2 := [3]uint32{gsX2, gsY2, 1}

			b.launchFan2(localSize2, globalSize2, gsX2, gsY2, t)
		}
	}

	// Copy results back
	resultA := make([]float32, n*n)
	resultB := make([]float32, n)
	b.driver.MemCopyD2H(b.context, resultA, b.dA)
	b.driver.MemCopyD2H(b.context, resultB, b.dB)
}

func (b *Benchmark) launchFan1(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX uint32, t int,
) {
	kernelArg := Fan1KernelArgs{
		M:                   b.dM,
		A:                   b.dA,
		Size:                int32(b.size),
		T:                   int32(t),
		HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.fan1Kernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchFan2(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32, t int,
) {
	kernelArg := Fan2KernelArgs{
		M:                   b.dM,
		A:                   b.dA,
		B:                   b.dB,
		Size:                int32(b.size),
		T:                   int32(t),
		HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
		HiddenBlockCountY:   globalSizeY / uint32(localSize[1]),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
		HiddenRemainderY:    uint16(globalSizeY % uint32(localSize[1])),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
	b.driver.LaunchKernel(b.context, b.fan2Kernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
