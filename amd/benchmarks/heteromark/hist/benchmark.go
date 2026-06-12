// Package hist implements the Histogram benchmark from Hetero-Mark.
// Single kernel: histogram_kernel with shared memory optimization.
// Input: array of unsigned ints in [0, numBins-1].
// Output: histogram bin counts.
package hist

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

// HistogramKernelArgs defines kernel arguments.
type HistogramKernelArgs struct {
	Data    driver.Ptr
	Hist    driver.Ptr
	N       int32
	NumBins int32
}

// Benchmark defines the Histogram benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	NumBins     int
	BlockSize   int
	MaxGrid     int

	hData []uint32
	hHist []uint32

	cpuHist []uint32

	dData driver.Ptr
	dHist driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a new Histogram benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()

	b.NumElements = 1048576
	b.NumBins = 256
	b.BlockSize = 256
	b.MaxGrid = 1024

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

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "histogram_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load histogram_kernel binary")
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

	b.hData = make([]uint32, b.NumElements)
	b.hHist = make([]uint32, b.NumBins)

	for i := 0; i < b.NumElements; i++ {
		b.hData[i] = uint32(rand.Intn(b.NumBins))
	}

	if b.useUnifiedMemory {
		b.dData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumElements*4))
		b.dHist = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumBins*4))
	} else {
		b.dData = b.driver.AllocateMemory(
			b.context, uint64(b.NumElements*4))
		b.dHist = b.driver.AllocateMemory(
			b.context, uint64(b.NumBins*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dData, b.hData)

	// Zero the histogram
	zeroHist := make([]uint32, b.NumBins)
	b.driver.MemCopyH2D(b.context, b.dHist, zeroHist)

	// Compute grid size
	grid := (b.NumElements + b.BlockSize - 1) / b.BlockSize
	if grid > b.MaxGrid {
		grid = b.MaxGrid
	}

	localSizeX := uint16(b.BlockSize)
	localSize := [3]uint16{localSizeX, 1, 1}
	gsX := uint32(grid * b.BlockSize)
	globalSize := [3]uint32{gsX, 1, 1}

	// Shared memory size: numBins * sizeof(uint32)
	sharedMemSize := driver.LocalPtr(b.NumBins * 4)

	b.launchHistogramKernel(globalSize, localSize, sharedMemSize)

	b.driver.MemCopyD2H(b.context, b.hHist, b.dHist)
}

func (b *Benchmark) launchHistogramKernel(
	globalSize [3]uint32, localSize [3]uint16,
	sharedMemSize driver.LocalPtr,
) {
	args := HistogramKernelArgs{
		Data:    b.dData,
		Hist:    b.dHist,
		N:       int32(b.NumElements),
		NumBins: int32(b.NumBins),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)
}

// Verify verifies the GPU histogram against a CPU reference.
func (b *Benchmark) Verify() {
	b.cpuHist = make([]uint32, b.NumBins)
	for i := 0; i < b.NumElements; i++ {
		b.cpuHist[b.hData[i]]++
	}

	errors := 0
	for i := 0; i < b.NumBins; i++ {
		if b.hHist[i] != b.cpuHist[i] {
			if errors < 5 {
				fmt.Fprintf(os.Stderr,
					"Mismatch at bin %d: GPU=%d CPU=%d\n",
					i, b.hHist[i], b.cpuHist[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d bins mismatched", errors)
	}

	fmt.Fprintf(os.Stderr, "Passed!\n")
}
