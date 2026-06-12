// Package ep implements the Heteromark Exclusive Prefix Scan benchmark.
// Two kernels: scan_block (per-block Blelloch scan) and add_block_prefix.
package ep

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ScanBlockKernelArgs defines kernel arguments.
type ScanBlockKernelArgs struct {
	Input               driver.Ptr
	Output              driver.Ptr
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

// AddBlockPrefixKernelArgs defines kernel arguments.
type AddBlockPrefixKernelArgs struct {
	Data                driver.Ptr
	BlockPrefix         driver.Ptr
	N                   int32
	ElemsPerBlock       int32
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

// Benchmark defines the Exclusive Prefix Scan benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	scanBlockKernel      *insts.KernelCodeObject
	addBlockPrefixKernel *insts.KernelCodeObject

	NumElements int

	hInput  []float32
	hOutput []float32

	dInput  driver.Ptr
	dOutput driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new EP benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
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

func (b *Benchmark) loadProgram() {
	b.scanBlockKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "scan_block")
	if b.scanBlockKernel == nil {
		log.Panic("Failed to load scan_block kernel binary")
	}

	b.addBlockPrefixKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "add_block_prefix")
	if b.addBlockPrefixKernel == nil {
		log.Panic("Failed to load add_block_prefix kernel binary")
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

	rand.Seed(42)
	b.hInput = make([]float32, n)
	b.hOutput = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hInput[i] = float32(rand.Intn(10))
	}

	bytes := uint64(n * 4)
	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dOutput = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dInput = b.driver.AllocateMemory(b.context, bytes)
		b.dOutput = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) exec() {
	n := b.NumElements

	b.driver.MemCopyH2D(b.context, b.dInput, b.hInput)

	blockSize := 256
	b.gpuScan(b.dInput, b.dOutput, n, blockSize)

	b.driver.MemCopyD2H(b.context, b.hOutput, b.dOutput)
}

// gpuScan performs a recursive exclusive prefix scan on the GPU.
func (b *Benchmark) gpuScan(dInput, dOutput driver.Ptr, n, blockSize int) {
	elemsPerBlock := blockSize * 2
	numBlocks := (n + elemsPerBlock - 1) / elemsPerBlock
	sharedBytes := elemsPerBlock * 4 // float32 = 4 bytes

	var dBlockSums driver.Ptr
	if numBlocks > 1 {
		if b.useUnifiedMemory {
			dBlockSums = b.driver.AllocateUnifiedMemory(
				b.context, uint64(numBlocks*4))
		} else {
			dBlockSums = b.driver.AllocateMemory(
				b.context, uint64(numBlocks*4))
		}
	}

	// Pass 1: per-block scan
	b.launchScanBlock(dInput, dOutput, dBlockSums, int32(n),
		numBlocks, blockSize, sharedBytes)

	if numBlocks > 1 {
		// Recursively scan block sums
		var dBlockPrefix driver.Ptr
		if b.useUnifiedMemory {
			dBlockPrefix = b.driver.AllocateUnifiedMemory(
				b.context, uint64(numBlocks*4))
		} else {
			dBlockPrefix = b.driver.AllocateMemory(
				b.context, uint64(numBlocks*4))
		}

		b.gpuScan(dBlockSums, dBlockPrefix, numBlocks, blockSize)

		// Add block prefixes back
		b.launchAddBlockPrefix(dOutput, dBlockPrefix,
			int32(n), int32(elemsPerBlock), blockSize)

		b.driver.FreeMemory(b.context, dBlockPrefix)
		b.driver.FreeMemory(b.context, dBlockSums)
	}
}

func (b *Benchmark) launchScanBlock(
	dInput, dOutput, dBlockSums driver.Ptr,
	n int32,
	numBlocks, blockSize, sharedBytes int,
) {
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{uint32(numBlocks * blockSize), 1, 1}

	kernelArg := ScanBlockKernelArgs{
		Input:               dInput,
		Output:              dOutput,
		BlockSums:           dBlockSums,
		N:                   n,
		HiddenBlockCountX:   uint32(numBlocks),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.scanBlockKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchAddBlockPrefix(
	dOutput, dBlockPrefix driver.Ptr,
	n, elemsPerBlock int32,
	blockSize int,
) {
	gsX := uint32(((int(n) - 1) / blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	kernelArg := AddBlockPrefixKernelArgs{
		Data:                dOutput,
		BlockPrefix:         dBlockPrefix,
		N:                   n,
		ElemsPerBlock:       elemsPerBlock,
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
	b.driver.LaunchKernel(b.context, b.addBlockPrefixKernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies the GPU results against a CPU reference.
func (b *Benchmark) Verify() {
	n := b.NumElements

	// CPU exclusive prefix scan
	ref := make([]float32, n)
	ref[0] = 0.0
	for i := 1; i < n; i++ {
		ref[i] = ref[i-1] + b.hInput[i-1]
	}

	maxErr := 0.0
	errCount := 0
	for i := 0; i < n; i++ {
		err := math.Abs(float64(b.hOutput[i]) - float64(ref[i]))
		denom := math.Abs(float64(ref[i])) + 1e-10
		relErr := err / denom
		if relErr > maxErr {
			maxErr = relErr
		}
		if relErr > 1e-3 {
			errCount++
		}
	}

	fmt.Fprintf(os.Stderr,
		"EP max relative error: %e, error count: %d / %d\n",
		maxErr, errCount, n)

	if errCount == 0 {
		fmt.Fprintf(os.Stderr, "Passed!\n")
	} else {
		log.Panicf("FAIL: %d elements with error > 1e-3", errCount)
	}
}
