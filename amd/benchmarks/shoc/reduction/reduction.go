// Package reduction implements the SHOC Reduction benchmark.
// Two-pass parallel sum reduction of a float array using shared memory.
package reduction

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
	Input               driver.Ptr
	Output              driver.Ptr
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

// Benchmark defines the reduction benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	reduceKernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hInput []float32

	dInput   driver.Ptr
	dPartial driver.Ptr
	dResult  driver.Ptr

	gpuResult float32
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new reduction benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.BlockSize = 256
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
	b.reduceKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "reduce_kernel")
	if b.reduceKernel == nil {
		log.Panic("Failed to load reduce_kernel binary")
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

	// Initialize input with known pattern: (i % 1000) * 0.001
	b.hInput = make([]float32, n)
	for i := 0; i < n; i++ {
		b.hInput[i] = float32(i%1000) * 0.001
	}

	elemsPerBlock := b.BlockSize * 2
	numBlocks := (n + elemsPerBlock - 1) / elemsPerBlock

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context,
			uint64(n*4))
		b.dPartial = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numBlocks*4))
		b.dResult = b.driver.AllocateUnifiedMemory(b.context,
			uint64(4))
	} else {
		b.dInput = b.driver.AllocateMemory(b.context,
			uint64(n*4))
		b.dPartial = b.driver.AllocateMemory(b.context,
			uint64(numBlocks*4))
		b.dResult = b.driver.AllocateMemory(b.context,
			uint64(4))
	}

	b.driver.MemCopyH2D(b.context, b.dInput, b.hInput)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	elemsPerBlock := blockSize * 2
	numBlocks := (n + elemsPerBlock - 1) / elemsPerBlock

	// Pass 1: reduce input to partial sums
	globalSize1 := [3]uint32{uint32(numBlocks * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	b.launchReduceKernel(b.dInput, b.dPartial, int32(n),
		globalSize1, localSize)

	// Pass 2: reduce partial sums to single value
	if numBlocks > 1 {
		numBlocks2 := (numBlocks + elemsPerBlock - 1) / elemsPerBlock
		if numBlocks2 == 1 {
			globalSize2 := [3]uint32{uint32(blockSize), 1, 1}
			b.launchReduceKernel(b.dPartial, b.dResult,
				int32(numBlocks), globalSize2, localSize)
		} else {
			// Multi-level: allocate temp, reduce partials, then final
			dTemp := b.driver.AllocateMemory(b.context,
				uint64(numBlocks2*4))

			globalSize2 := [3]uint32{uint32(numBlocks2 * blockSize), 1, 1}
			b.launchReduceKernel(b.dPartial, dTemp,
				int32(numBlocks), globalSize2, localSize)

			globalSize3 := [3]uint32{uint32(blockSize), 1, 1}
			b.launchReduceKernel(dTemp, b.dResult,
				int32(numBlocks2), globalSize3, localSize)
		}
	} else {
		// Single block: copy partial[0] to result
		tmpBuf := make([]float32, 1)
		b.driver.MemCopyD2H(b.context, tmpBuf, b.dPartial)
		b.driver.MemCopyH2D(b.context, b.dResult, tmpBuf)
	}

	// Read result back
	result := make([]float32, 1)
	b.driver.MemCopyD2H(b.context, result, b.dResult)
	b.gpuResult = result[0]
}

func (b *Benchmark) launchReduceKernel(
	input, output driver.Ptr,
	n int32,
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	args := KernelArgs{
		Input:             input,
		Output:            output,
		N:                 n,
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
		b.reduceKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	cpuSum := float64(0)
	for i := 0; i < b.NumElements; i++ {
		cpuSum += float64(b.hInput[i])
	}

	gpuSum := float64(b.gpuResult)
	relErr := math.Abs(gpuSum-cpuSum) / (math.Abs(cpuSum) + 1e-10)

	if relErr < 1e-3 {
		log.Printf("Passed! GPU sum: %f, CPU sum: %f, relative error: %e",
			b.gpuResult, cpuSum, relErr)
	} else {
		log.Panicf("Mismatch! GPU sum: %f, CPU sum: %f, relative error: %e",
			b.gpuResult, cpuSum, relErr)
	}
}
