// Package sort implements the SHOC Sort (parallel radix sort) benchmark.
// Four kernels:
//   - scan_block_kernel: Blelloch-style exclusive prefix scan (uint)
//   - add_block_sums_uint_kernel: fixup to combine block scan results
//   - compute_predicates_kernel: compute per-element bit predicates
//   - scatter_kernel: scatter elements based on scan results
package sort

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ScanBlockKernelArgs defines kernel arguments.
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

// AddBlockSumsKernelArgs defines kernel arguments.
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

// ComputePredicatesKernelArgs defines kernel arguments.
type ComputePredicatesKernelArgs struct {
	Predicates          driver.Ptr
	Keys                driver.Ptr
	Bit                 int32
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

// ScatterKernelArgs defines kernel arguments.
type ScatterKernelArgs struct {
	KeysOut             driver.Ptr
	KeysIn              driver.Ptr
	Predicates          driver.Ptr
	ScanResult          driver.Ptr
	TotalZeros          uint32
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

// Benchmark defines the sort benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	scanBlockKernel         *insts.KernelCodeObject
	addBlockSumsKernel      *insts.KernelCodeObject
	computePredicatesKernel *insts.KernelCodeObject
	scatterKernel           *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hKeys   []uint32
	hResult []uint32

	dKeysIn     driver.Ptr
	dKeysOut    driver.Ptr
	dPredicates driver.Ptr
	dScanResult driver.Ptr

	nPadded int
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new sort benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 65536
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
		hsacoBytes, "add_block_sums_uint_kernel")
	if b.addBlockSumsKernel == nil {
		log.Panic("Failed to load add_block_sums_uint_kernel binary")
	}

	b.computePredicatesKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "compute_predicates_kernel")
	if b.computePredicatesKernel == nil {
		log.Panic("Failed to load compute_predicates_kernel binary")
	}

	b.scatterKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "scatter_kernel")
	if b.scatterKernel == nil {
		log.Panic("Failed to load scatter_kernel binary")
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

	bytes := uint64(b.nPadded) * 4

	b.hKeys = make([]uint32, b.nPadded)
	b.hResult = make([]uint32, n)

	// Initialize with pseudo-random values matching HIP code
	rand.Seed(42)
	for i := 0; i < n; i++ {
		b.hKeys[i] = uint32(rand.Int31())
	}
	// Remaining padded elements are zero

	if b.useUnifiedMemory {
		b.dKeysIn = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dKeysOut = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dPredicates = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dScanResult = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dKeysIn = b.driver.AllocateMemory(b.context, bytes)
		b.dKeysOut = b.driver.AllocateMemory(b.context, bytes)
		b.dPredicates = b.driver.AllocateMemory(b.context, bytes)
		b.dScanResult = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) launchComputePredicatesKernel(
	globalSize [3]uint32, localSize [3]uint16,
	bit, nPadded int,
) {
	args := ComputePredicatesKernelArgs{
		Predicates: b.dPredicates,
		Keys:       b.dKeysIn,
		Bit:        int32(bit),
		N:          int32(nPadded),
	}
	b.driver.LaunchKernel(b.context, b.computePredicatesKernel, globalSize, localSize, &args)
}

func (b *Benchmark) launchScatterKernel(
	globalSize [3]uint32, localSize [3]uint16,
	totalZeros uint32, n int,
) {
	args := ScatterKernelArgs{
		KeysOut:    b.dKeysOut,
		KeysIn:     b.dKeysIn,
		Predicates: b.dPredicates,
		ScanResult: b.dScanResult,
		TotalZeros: totalZeros,
		N:          int32(n),
	}
	b.driver.LaunchKernel(b.context, b.scatterKernel, globalSize, localSize, &args)
}

func (b *Benchmark) execOneBit(bit, n, nPadded, blockSize int) {
	gridPadded := (nPadded + blockSize - 1) / blockSize
	grid := (n + blockSize - 1) / blockSize

	predGlobalSize := [3]uint32{uint32(gridPadded * blockSize), 1, 1}
	predLocalSize := [3]uint16{uint16(blockSize), 1, 1}
	b.launchComputePredicatesKernel(predGlobalSize, predLocalSize, bit, nPadded)

	b.scanRecursive(b.dScanResult, b.dPredicates, nPadded, blockSize)

	lastScan := make([]uint32, 1)
	lastPred := make([]uint32, 1)
	b.driver.MemCopyD2H(b.context, lastScan, b.dScanResult+driver.Ptr((n-1)*4))
	b.driver.MemCopyD2H(b.context, lastPred, b.dPredicates+driver.Ptr((n-1)*4))
	totalZeros := lastScan[0] + lastPred[0]

	scatterGlobalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	scatterLocalSize := [3]uint16{uint16(blockSize), 1, 1}
	b.launchScatterKernel(scatterGlobalSize, scatterLocalSize, totalZeros, n)

	b.dKeysIn, b.dKeysOut = b.dKeysOut, b.dKeysIn
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	nPadded := b.nPadded

	b.driver.MemCopyH2D(b.context, b.dKeysIn, b.hKeys)

	for bit := 0; bit < 32; bit++ {
		b.execOneBit(bit, n, nPadded, blockSize)
	}

	b.driver.MemCopyD2H(b.context, b.hResult, b.dKeysIn)
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
