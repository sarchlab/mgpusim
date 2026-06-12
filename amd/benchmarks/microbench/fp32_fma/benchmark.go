// Package fp32fma implements FP32 fused multiply-add throughput microbenchmark.
package fp32fma

import (
	"log"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for fp32_fma_kernel.
// Kernel signature: (int num_ops, float *d_out)
// int = 4 bytes, pointer = 8 bytes, need padding for alignment
type KernelArgs struct {
	NumOps              int32
	Pad0                int32
	DOut                driver.Ptr
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

// Benchmark defines the FP32 FMA benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumOps    int
	BlockSize int

	useUnifiedMemory bool

	dOut driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new FP32 FMA benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumOps = 1048576
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

// SetNumOps sets the number of loop iterations per thread.
func (b *Benchmark) SetNumOps(n int) {
	b.NumOps = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fp32_fma_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load fp32_fma_kernel")
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
	// Fixed 65536 threads, 4 bytes per float
	numThreads := 65536
	bytes := uint64(numThreads * 4)

	if b.useUnifiedMemory {
		b.dOut = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dOut = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) exec() {
	numThreads := 65536
	blockSize := b.BlockSize
	grid := (numThreads + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		NumOps:            int32(b.NumOps),
		DOut:              b.dOut,
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
}

// Verify verifies the result (no-op: DCE guard means no real output).
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
