// Package fp32throughput implements the fp32_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/fp32_throughput) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Each work-item runs a chain of fused multiply-add (FMA) operations on
// register-resident floats using four independent accumulators. The kernel
// is memory-traffic free except for a single checksum write from work-item
// (0,0), which is what Verify() reproduces on the CPU. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300A configuration).
package fp32throughput

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// FMA multiplier / addend constants, kept identical to the HIP kernel so the
// CPU reference in Verify() matches the GPU result bit-for-bit in spirit.
const (
	fmaMul float32 = 1.0000001
	fmaAdd float32 = 0.0000001
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// One 8-byte global_buffer pointer followed by two 4-byte by_value scalars,
// packed with no padding (mgpusim serializes args with binary.Write, which does
// not insert alignment padding; all fields are naturally aligned here so
// kernarg_segment_size = 16). The kernel reads only blockIdx.x / threadIdx.x
// plus these explicit args -- block size is passed as ThreadsPerBlock rather
// than read from blockDim.x, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Out             driver.Ptr // offset 0, size 8 (global_buffer)
	FmasPerThread   int32      // offset 8, size 4 (by_value)
	ThreadsPerBlock int32      // offset 12, size 4 (by_value)
}

// Benchmark defines the fp32_throughput benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// NumBlocks is the number of thread blocks to launch.
	NumBlocks int
	// FmasPerThread is the number of FMA iterations per thread. It is rounded
	// down to a multiple of 4 to match the kernel's 4-way unrolled loop.
	FmasPerThread int
	// ThreadsPerBlock is the work-group (block) size. Defaults to 256; must be
	// <= 1024 (the gfx942 hardware maximum). The kernel reads blockDim.x, so any
	// value launches correctly and the sweep can match the ground-truth
	// threads_per_block dimension.
	ThreadsPerBlock int

	gOut driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp32_throughput benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fp32_fma_kernel")
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
		log.Panic("the fp32_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// fmasPerThread returns the effective FMA count, rounded down to a multiple
// of 4 (matching the kernel's unrolled loop), with sane defaults.
func (b *Benchmark) fmasPerThread() int32 {
	f := b.FmasPerThread
	if f <= 0 {
		f = 256
	}
	f = (f / 4) * 4
	if f == 0 {
		f = 4
	}
	return int32(f)
}

func (b *Benchmark) numBlocks() int {
	if b.NumBlocks <= 0 {
		return 4
	}
	return b.NumBlocks
}

func (b *Benchmark) threadsPerBlock() int {
	if b.ThreadsPerBlock <= 0 {
		return 256
	}
	return b.ThreadsPerBlock
}

func (b *Benchmark) numThreads() int {
	return b.numBlocks() * b.threadsPerBlock()
}

func (b *Benchmark) initMem() {
	numElem := b.numThreads()
	outBytes := uint64(numElem * 4)

	if b.useUnifiedMemory {
		b.gOut = b.driver.AllocateUnifiedMemory(b.context, outBytes)
	} else {
		b.gOut = b.driver.AllocateMemory(b.context, outBytes)
	}

	// Initialize the output buffer to a known value.
	b.driver.MemCopyH2D(b.context, b.gOut, make([]float32, numElem))
}

func (b *Benchmark) exec() {
	numBlocks := b.numBlocks()
	tpb := b.threadsPerBlock()
	globalX := uint32(numBlocks * tpb)

	args := KernelArgs{
		Out:             b.gOut,
		FmasPerThread:   b.fmasPerThread(),
		ThreadsPerBlock: int32(tpb),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(tpb), 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation.
//
// Each thread writes out[tid] = a0+a1+a2+a3, where the four accumulators start
// at (1 + threadIdx.x*0.001) plus 0/0.1/0.2/0.3 and each is iterated
// fmas_per_thread/4 times through a = a*mul + add. Since threadIdx.x is the
// lane index within a block, the reference depends only on threadIdx.x and is
// reproduced exactly here using float32 arithmetic.
func (b *Benchmark) Verify() {
	numElem := b.numThreads()
	gpu := make([]float32, numElem)
	b.driver.MemCopyD2H(b.context, gpu, b.gOut)

	iters := int(b.fmasPerThread()) / 4
	tpb := b.threadsPerBlock()

	// The result for a thread depends only on threadIdx.x (== tid % block size),
	// so precompute one reference value per lane.
	ref := make([]float32, tpb)
	for lane := 0; lane < tpb; lane++ {
		a0 := float32(1.0) + float32(lane)*0.001
		a1 := a0 + 0.1
		a2 := a0 + 0.2
		a3 := a0 + 0.3

		for i := 0; i < iters; i++ {
			a0 = a0*fmaMul + fmaAdd
			a1 = a1*fmaMul + fmaAdd
			a2 = a2*fmaMul + fmaAdd
			a3 = a3*fmaMul + fmaAdd
		}
		ref[lane] = a0 + a1 + a2 + a3
	}

	for tid := 0; tid < numElem; tid++ {
		want := float64(ref[tid%tpb])
		got := float64(gpu[tid])

		denom := math.Abs(want)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(want-got)/denom > 1e-3 {
			log.Fatalf("At thread %d, expected %f, but got %f.\n",
				tid, want, got)
		}
	}

	log.Printf("Passed!\n")
}
