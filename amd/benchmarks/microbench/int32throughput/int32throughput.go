// Package int32throughput implements the int32_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/int32_throughput) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Each thread runs a long chain of INT32 multiply-add operations
// (a = a*mul + add) entirely in registers and then writes its accumulated
// result to out[globalThreadId]. The dominant compute is the register-only
// MAD chain (the original throughput microbenchmark), while the per-thread
// store makes the result fully verifiable on the host with int32
// wraparound.
//
// The kernel uses a constant block size (blockSize) instead of blockDim.x,
// so the compiler emits no hidden ABI arguments; the launcher therefore
// must use the same block size. The kernel binary is compiled for gfx942
// only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300A configuration).
package int32throughput

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize must match the BLOCK constant compiled into the gfx942 kernel
// (native/int32_throughput.cpp). The kernel computes the global thread id
// as blockIdx.x * blockSize + threadIdx.x.
const blockSize = 64

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 12): one 8-byte global_buffer pointer
// followed by one 4-byte by_value int32, packed with no padding (mgpusim
// serializes args with binary.Write, which does not insert alignment
// padding). The kernel uses only threadIdx.x / blockIdx.x (no
// blockDim/gridDim), so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Out           driver.Ptr // offset 0
	MadsPerThread int32      // offset 8
}

// Benchmark defines the int32_throughput benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// MadsPerThread is the number of multiply-add operations per thread.
	// It is rounded down to a multiple of 4 (and forced to at least 4),
	// matching the HIP host program.
	MadsPerThread int
	// NumBlocks is the number of work-groups (blocks) in the 1D grid. The
	// work-group size is fixed at blockSize.
	NumBlocks int

	madsPerThread int
	numThreads    int

	gOut driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new int32_throughput benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "int32_mad_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the int32_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 16
	}
	if b.MadsPerThread <= 0 {
		b.MadsPerThread = 4096
	}

	// Match the HIP host: round down to a multiple of 4, min 4.
	b.madsPerThread = (b.MadsPerThread / 4) * 4
	if b.madsPerThread == 0 {
		b.madsPerThread = 4
	}

	b.numThreads = b.NumBlocks * blockSize

	if b.useUnifiedMemory {
		b.gOut = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.numThreads*4))
	} else {
		b.gOut = b.driver.AllocateMemory(
			b.context, uint64(b.numThreads*4))
	}
}

func (b *Benchmark) exec() {
	globalX := uint32(b.numThreads)

	args := KernelArgs{
		Out:           b.gOut,
		MadsPerThread: int32(b.madsPerThread),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// cpuReferenceForThread reproduces, exactly, the value a thread with the
// given threadIdx.x writes. The initial accumulators are
// a0=1+threadIdx.x, a1=a0+111, a2=a0+222, a3=a0+333; all arithmetic is
// int32 with wraparound.
func (b *Benchmark) cpuReferenceForThread(threadIdxX int) int32 {
	a0 := int32(1 + threadIdxX)
	a1 := a0 + 111
	a2 := a0 + 222
	a3 := a0 + 333

	const mul = int32(3)
	const add = int32(1)

	for i := 0; i < b.madsPerThread; i += 4 {
		a0 = a0*mul + add
		a1 = a1*mul + add
		a2 = a2*mul + add
		a3 = a3*mul + add
	}

	return a0 + a1 + a2 + a3
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	gpuOut := make([]int32, b.numThreads)
	b.driver.MemCopyD2H(b.context, gpuOut, b.gOut)

	for tid := 0; tid < b.numThreads; tid++ {
		threadIdxX := tid % blockSize
		ref := b.cpuReferenceForThread(threadIdxX)
		got := gpuOut[tid]

		if ref != got {
			log.Fatalf("Verification failed at thread %d: "+
				"expected %d, but got %d.\n", tid, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
