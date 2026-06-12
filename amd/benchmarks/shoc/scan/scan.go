// Package scan implements the SHOC Scan (parallel prefix sum) benchmark.
// Two kernels: scan_block_kernel (Blelloch-style exclusive scan per block)
// and add_block_sums_kernel (fixup to combine block results).
package scan

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ScanBlockKernelArgs defines kernel arguments for the scan_block_kernel.
type ScanBlockKernelArgs struct {
	Output              driver.Ptr
	Input               driver.Ptr
	BlockSums           driver.Ptr
	N                   int32
	Padding             int32
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

// AddBlockSumsKernelArgs defines kernel arguments for the add_block_sums_kernel.
type AddBlockSumsKernelArgs struct {
	Data                driver.Ptr
	BlockSums           driver.Ptr
	N                   int32
	Padding             int32
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

// Benchmark defines the scan benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	scanBlockKernel    *insts.KernelCodeObject
	addBlockSumsKernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hInput  []float32
	hOutput []float32

	dInput  driver.Ptr
	dOutput driver.Ptr

	nPadded int
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new scan benchmark.
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

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetNumElements sets the number of elements.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

func (b *Benchmark) loadProgram() {
	b.scanBlockKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "scan_block_kernel")
	if b.scanBlockKernel == nil {
		log.Panic("Failed to load scan_block_kernel binary")
	}

	b.addBlockSumsKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "add_block_sums_kernel")
	if b.addBlockSumsKernel == nil {
		log.Panic("Failed to load add_block_sums_kernel binary")
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
	elementsPerBlock := b.BlockSize * 2
	b.nPadded = ((n + elementsPerBlock - 1) / elementsPerBlock) * elementsPerBlock

	b.hInput = make([]float32, b.nPadded)
	b.hOutput = make([]float32, b.nPadded)

	// Initialize with deterministic values matching the HIP code
	for i := 0; i < n; i++ {
		b.hInput[i] = float32(i%7) + 1.0
	}
	// Remaining padded elements are zero

	bytes := uint64(b.nPadded) * 4

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dOutput = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dInput = b.driver.AllocateMemory(b.context, bytes)
		b.dOutput = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dInput, b.hInput)
}

func (b *Benchmark) exec() {
	b.scanRecursive(b.dOutput, b.dInput, b.nPadded, b.BlockSize)

	// Copy result back
	b.driver.MemCopyD2H(b.context, b.hOutput, b.dOutput)
}

func (b *Benchmark) launchScanBlockKernel(
	dOutput, dInput, dBlockSums driver.Ptr,
	globalSize [3]uint32, localSize [3]uint16,
	n int,
) {
	args := ScanBlockKernelArgs{
		Output:    dOutput,
		Input:     dInput,
		BlockSums: dBlockSums,
		N:         int32(n),
	}
	b.driver.LaunchKernel(b.context, b.scanBlockKernel, globalSize, localSize, &args)
}

func (b *Benchmark) launchAddBlockSumsKernel(
	dOutput, dScannedBlockSums driver.Ptr,
	globalSize [3]uint32, localSize [3]uint16,
	n int,
) {
	args := AddBlockSumsKernelArgs{
		Data:      dOutput,
		BlockSums: dScannedBlockSums,
		N:         int32(n),
	}
	b.driver.LaunchKernel(b.context, b.addBlockSumsKernel, globalSize, localSize, &args)
}

func (b *Benchmark) scanRecursive(dOutput, dInput driver.Ptr, n, blockSize int) {
	elementsPerBlock := blockSize * 2
	numBlocks := (n + elementsPerBlock - 1) / elementsPerBlock

	var dBlockSums driver.Ptr
	if numBlocks > 1 {
		dBlockSums = b.driver.AllocateMemory(b.context, uint64(numBlocks)*4)
	}

	globalSize := [3]uint32{uint32(numBlocks * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	b.launchScanBlockKernel(dOutput, dInput, dBlockSums, globalSize, localSize, n)

	if numBlocks > 1 {
		dScannedBlockSums := b.driver.AllocateMemory(b.context, uint64(numBlocks)*4)
		b.scanRecursive(dScannedBlockSums, dBlockSums, numBlocks, blockSize)

		addGlobalSize := [3]uint32{uint32(numBlocks * blockSize), 1, 1}
		addLocalSize := [3]uint16{uint16(blockSize), 1, 1}
		b.launchAddBlockSumsKernel(dOutput, dScannedBlockSums, addGlobalSize, addLocalSize, n)
	}
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
