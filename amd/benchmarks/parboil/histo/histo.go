// Package histo implements the Parboil Histogram benchmark.
// Two kernels: clear_bins (zero the histogram) and histogram (compute histogram).
package histo

import (
	"fmt"
	"log"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ClearBinsKernelArgs defines kernel arguments.
type ClearBinsKernelArgs struct {
	Bins                driver.Ptr
	NumBins             int32
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

// HistogramKernelArgs defines kernel arguments.
type HistogramKernelArgs struct {
	Input               driver.Ptr
	Bins                driver.Ptr
	NumElements         int32
	NumBins             int32
	Saturation          uint32
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

// Benchmark defines the Parboil Histogram benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	clearBinsKernel *insts.KernelCodeObject
	histogramKernel *insts.KernelCodeObject

	NumElements int
	NumBins     int
	BlockSize   int
	Saturation  uint32

	useUnifiedMemory bool

	hInput []uint32
	hBins  []uint32

	cpuBins []uint32

	dInput driver.Ptr
	dBins  driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Parboil Histogram benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.NumBins = 256
	b.BlockSize = 256
	b.Saturation = 255
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
	b.clearBinsKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "clear_bins")
	if b.clearBinsKernel == nil {
		log.Panic("Failed to load clear_bins kernel binary")
	}

	b.histogramKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "histogram")
	if b.histogramKernel == nil {
		log.Panic("Failed to load histogram kernel binary")
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
	rand.Seed(42)

	b.hInput = make([]uint32, b.NumElements)
	b.hBins = make([]uint32, b.NumBins)

	for i := 0; i < b.NumElements; i++ {
		b.hInput[i] = uint32(rand.Intn(b.NumBins))
	}
	for i := 0; i < b.NumBins; i++ {
		b.hBins[i] = 0
	}

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumElements*4))
		b.dBins = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumBins*4))
	} else {
		b.dInput = b.driver.AllocateMemory(
			b.context, uint64(b.NumElements*4))
		b.dBins = b.driver.AllocateMemory(
			b.context, uint64(b.NumBins*4))
	}

	b.driver.MemCopyH2D(b.context, b.dInput, b.hInput)
	b.driver.MemCopyH2D(b.context, b.dBins, b.hBins)
}

func (b *Benchmark) exec() {
	blockSize := b.BlockSize

	// Launch clear_bins kernel
	clearBlocks := (b.NumBins + blockSize - 1) / blockSize
	clearGlobalSize := [3]uint32{uint32(clearBlocks * blockSize), 1, 1}
	clearLocalSize := [3]uint16{uint16(blockSize), 1, 1}

	b.launchClearBins(clearGlobalSize, clearLocalSize)

	// Launch histogram kernel
	histBlocks := (b.NumElements + blockSize - 1) / blockSize
	histGlobalSize := [3]uint32{uint32(histBlocks * blockSize), 1, 1}
	histLocalSize := [3]uint16{uint16(blockSize), 1, 1}

	b.launchHistogram(histGlobalSize, histLocalSize)

	b.driver.MemCopyD2H(b.context, b.hBins, b.dBins)
}

func (b *Benchmark) launchClearBins(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := ClearBinsKernelArgs{
		Bins:              b.dBins,
		NumBins:           int32(b.NumBins),
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context,
		b.clearBinsKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) launchHistogram(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := HistogramKernelArgs{
		Input:             b.dInput,
		Bins:              b.dBins,
		NumElements:       int32(b.NumElements),
		NumBins:           int32(b.NumBins),
		Saturation:        b.Saturation,
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context,
		b.histogramKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the GPU histogram against a CPU reference.
func (b *Benchmark) Verify() {
	b.cpuBins = make([]uint32, b.NumBins)
	for i := 0; i < b.NumElements; i++ {
		bin := b.hInput[i] % uint32(b.NumBins)
		if b.Saturation == 0 || b.cpuBins[bin] < b.Saturation {
			b.cpuBins[bin]++
		}
	}

	errors := 0
	for i := 0; i < b.NumBins; i++ {
		if b.hBins[i] != b.cpuBins[i] {
			if errors < 5 {
				fmt.Fprintf(os.Stderr,
					"Mismatch at bin %d: GPU=%d CPU=%d\n",
					i, b.hBins[i], b.cpuBins[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d bins mismatched", errors)
	}

	fmt.Fprintf(os.Stderr, "Passed!\n")
}
