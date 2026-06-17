// Package fp64throughput implements the fp64_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/fp64_throughput) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Each work-item runs a long chain of FP64 fused multiply-adds (FMA) over
// four independent accumulators, then stores the sum of its accumulators
// to out[globalThreadId]. Its FP64 operands (the four seeds and the FMA
// multiplier/addend) are read from a per-thread slice of an input buffer,
// so the kernel measures FP64 arithmetic throughput. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300A config).
package fp64throughput

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the fixed work-group size (BLOCK_SIZE in the kernel). It is a
// compile-time constant in the kernel so blockDim.x is never read, which
// keeps the kernel free of hidden ABI arguments.
const blockSize = 64

// stride is the number of doubles per thread in the input buffer (STRIDE in
// the kernel). It is a power of two so the kernel forms the load address
// with a supported shift-add instead of a 64-bit integer multiply; see
// native/fp64_throughput.cpp for the full rationale. Only the first six
// slots carry data (seeds a0..a3, then mul, add); slots 6..7 are padding.
const stride = 8

// FMA chain constants, matching what the driver writes into the input
// buffer for every thread.
const (
	seedBase = 1.001
	fmaMul   = 1.0000000001
	fmaAdd   = 0.0000000001
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 20): two 8-byte global_buffer pointers
// (out, in) followed by one 4-byte by_value int, packed with no padding
// (mgpusim serializes args with binary.Write, which does not insert
// alignment padding). The kernel reads only blockIdx.x / threadIdx.x (block
// size is a compile-time constant), so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Out           driver.Ptr // offset 0
	In            driver.Ptr // offset 8
	FmasPerThread int32      // offset 16
}

// Benchmark defines the fp64_throughput benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// NumBlocks is the number of work-groups (grid dim in blocks). Each
	// block has blockSize (64) work-items.
	NumBlocks int
	// FmasPerThread is the number of FMA iterations per work-item. It is
	// rounded down to a multiple of 4 (matching the host program), with a
	// minimum of 4. Each iteration performs 4 FP64 FMAs (one per
	// accumulator), so total FP64 FLOPs = numThreads * FmasPerThread * 2.
	FmasPerThread int

	numThreads int
	in         []float64
	gIn        driver.Ptr
	gOut       driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp64_throughput benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fp64_fma_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. fp64_throughput uses a single GPU.
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
		log.Panic("the fp64_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) normalizeParams() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 4
	}
	if b.FmasPerThread <= 0 {
		b.FmasPerThread = 16
	}
	// Match the host program: round down to a multiple of 4, minimum 4.
	b.FmasPerThread = (b.FmasPerThread / 4) * 4
	if b.FmasPerThread == 0 {
		b.FmasPerThread = 4
	}

	b.numThreads = b.NumBlocks * blockSize
}

// seedFor returns the seed for accumulator acc (0..3) of work-item gid.
// Every work-item uses the same seeds (matching the kernel), so gid is
// unused; it is kept so the layout is explicit and easy to extend.
func seedFor(acc int) float64 {
	a0 := seedBase
	switch acc {
	case 0:
		return a0
	case 1:
		return a0 + 0.1
	case 2:
		return a0 + 0.2
	default:
		return a0 + 0.3
	}
}

func (b *Benchmark) initMem() {
	b.normalizeParams()

	// Build the per-thread input buffer: stride doubles per thread, with
	// slots [0..3]=seeds, [4]=mul, [5]=add, [6..7]=padding.
	b.in = make([]float64, b.numThreads*stride)
	for t := 0; t < b.numThreads; t++ {
		base := t * stride
		b.in[base+0] = seedFor(0)
		b.in[base+1] = seedFor(1)
		b.in[base+2] = seedFor(2)
		b.in[base+3] = seedFor(3)
		b.in[base+4] = fmaMul
		b.in[base+5] = fmaAdd
	}

	inBytes := uint64(len(b.in) * 8)
	outBytes := uint64(b.numThreads * 8)
	if b.useUnifiedMemory {
		b.gIn = b.driver.AllocateUnifiedMemory(b.context, inBytes)
		b.gOut = b.driver.AllocateUnifiedMemory(b.context, outBytes)
	} else {
		b.gIn = b.driver.AllocateMemory(b.context, inBytes)
		b.gOut = b.driver.AllocateMemory(b.context, outBytes)
	}

	b.driver.MemCopyH2D(b.context, b.gIn, b.in)
	// Initialize the output to zero so a missing store is detectable.
	b.driver.MemCopyH2D(b.context, b.gOut, make([]float64, b.numThreads))
}

func (b *Benchmark) exec() {
	globalX := uint32(b.numThreads)

	args := KernelArgs{
		Out:           b.gOut,
		In:            b.gIn,
		FmasPerThread: int32(b.FmasPerThread),
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

// cpuReference reproduces, in pure Go, the value the kernel stores. Every
// work-item computes the same value because every thread reads identical
// seeds and constants from the input buffer.
func (b *Benchmark) cpuReference() float64 {
	a0 := seedFor(0)
	a1 := seedFor(1)
	a2 := seedFor(2)
	a3 := seedFor(3)

	for i := 0; i < b.FmasPerThread; i += 4 {
		a0 = math.FMA(a0, fmaMul, fmaAdd)
		a1 = math.FMA(a1, fmaMul, fmaAdd)
		a2 = math.FMA(a2, fmaMul, fmaAdd)
		a3 = math.FMA(a3, fmaMul, fmaAdd)
	}

	return a0 + a1 + a2 + a3
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	gpuOut := make([]float64, b.numThreads)
	b.driver.MemCopyD2H(b.context, gpuOut, b.gOut)

	ref := b.cpuReference()
	denom := math.Abs(ref)
	if denom < 1.0 {
		denom = 1.0
	}

	for gid := 0; gid < b.numThreads; gid++ {
		got := gpuOut[gid]
		if math.Abs(ref-got)/denom > 1e-3 {
			log.Fatalf("At gid %d, expected %.12f, but got %.12f.\n",
				gid, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
