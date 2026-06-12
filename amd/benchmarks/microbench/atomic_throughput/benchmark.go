// Package atomicthroughput implements atomic operation throughput microbenchmark.
package atomicthroughput

import (
	"log"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for atomic_throughput_kernel.
// Kernel signature: (int *counters, int num_counters, int num_atomics_per_thread)
type KernelArgs struct {
	Counters            driver.Ptr
	NumCounters         int32
	NumAtomicsPerThread int32
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

// Benchmark defines the atomic throughput benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements         int
	NumCounters         int
	NumAtomicsPerThread int
	BlockSize           int

	useUnifiedMemory bool

	hCounters []int32

	dCounters driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new atomic throughput benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 65536
	b.NumCounters = 1
	b.NumAtomicsPerThread = 1024
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

// SetNumElements sets the number of threads.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

// SetNumCounters sets the number of atomic counters.
func (b *Benchmark) SetNumCounters(n int) {
	b.NumCounters = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "atomic_throughput_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load atomic_throughput_kernel")
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
	b.hCounters = make([]int32, b.NumCounters)

	bytes := uint64(b.NumCounters * 4)

	if b.useUnifiedMemory {
		b.dCounters = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dCounters = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dCounters, b.hCounters)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		Counters:            b.dCounters,
		NumCounters:         int32(b.NumCounters),
		NumAtomicsPerThread: int32(b.NumAtomicsPerThread),
		HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hCounters, b.dCounters)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	totalCount := int32(0)
	for i := 0; i < b.NumCounters; i++ {
		totalCount += b.hCounters[i]
	}

	expected := int32(b.NumElements * b.NumAtomicsPerThread)
	if totalCount != expected {
		log.Panicf("FAIL: total count %d, expected %d", totalCount, expected)
	}
	log.Printf("Passed!")
}
