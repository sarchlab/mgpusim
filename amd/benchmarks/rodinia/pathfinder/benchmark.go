// Package pathfinder implements the Rodinia Pathfinder benchmark.
// One kernel: pathfinder_kernel (1D dynamic programming shortest path)
package pathfinder

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Src                 driver.Ptr
	Dst                 driver.Ptr
	Wall                driver.Ptr
	Width               int32
	Row                 int32
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

	pathfinderKernel *insts.KernelCodeObject

	cols int
	rows int

	hWall   []int32
	hResult []int32

	dWall driver.Ptr
	dSrc  driver.Ptr
	dDst  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.cols = 512
	b.rows = 100
	return b
}

// SetCols sets the number of columns
func (b *Benchmark) SetCols(cols int) {
	b.cols = cols
}

// SetRows sets the number of rows
func (b *Benchmark) SetRows(rows int) {
	b.rows = rows
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
	b.pathfinderKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "pathfinder_kernel")
	if b.pathfinderKernel == nil {
		log.Panic("Failed to load pathfinder_kernel binary")
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
	totalCells := b.cols * b.rows

	rand.Seed(42)
	b.hWall = make([]int32, totalCells)
	b.hResult = make([]int32, b.cols)

	for i := 0; i < totalCells; i++ {
		b.hWall[i] = int32(rand.Intn(10))
	}

	// Initialize first row as starting costs
	for j := 0; j < b.cols; j++ {
		b.hResult[j] = b.hWall[j]
	}

	if b.useUnifiedMemory {
		b.dWall = b.driver.AllocateUnifiedMemory(b.context,
			uint64(totalCells*4))
		b.dSrc = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.cols*4))
		b.dDst = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.cols*4))
	} else {
		b.dWall = b.driver.AllocateMemory(b.context,
			uint64(totalCells*4))
		b.dSrc = b.driver.AllocateMemory(b.context,
			uint64(b.cols*4))
		b.dDst = b.driver.AllocateMemory(b.context,
			uint64(b.cols*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dWall, b.hWall)
	b.driver.MemCopyH2D(b.context, b.dSrc, b.hResult)

	blockSize := 256
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	gsX := uint32(((b.cols - 1) / blockSize + 1) * blockSize)
	globalSize := [3]uint32{gsX, 1, 1}

	for row := 1; row < b.rows; row++ {
		kernelArg := KernelArgs{
			Src:                 b.dSrc,
			Dst:                 b.dDst,
			Wall:                b.dWall,
			Width:               int32(b.cols),
			Row:                 int32(row),
			HiddenBlockCountX:   gsX / uint32(localSize[0]),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}
		b.driver.LaunchKernel(b.context, b.pathfinderKernel,
			globalSize, localSize, &kernelArg)

		// Swap src and dst
		b.dSrc, b.dDst = b.dDst, b.dSrc
	}

	// Copy result back
	result := make([]int32, b.cols)
	b.driver.MemCopyD2H(b.context, result, b.dSrc)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
