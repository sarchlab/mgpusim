// Package fp64throughput implements the fp64_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/fp64_throughput) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Each work-item runs a long chain of FP64 fused multiply-adds (FMA) over
// four independent accumulators, then stores the sum of its accumulators
// to out[globalThreadId]. Its FP64 operands (the four seeds and the FMA
// multiplier/addend) are read from a per-thread slice of an input buffer,
// so the kernel measures FP64 arithmetic throughput. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X config).
package fp64throughput

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// defaultThreadsPerBlock is the work-group size used when ThreadsPerBlock is
// unset. The block size is a runtime kernel argument (passed explicitly
// because blockDim.x reads back as 0 in this model), so the calibration can
// sweep it.
const defaultThreadsPerBlock = 256

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
// metadata (kernarg_segment_size = 24): two 8-byte global_buffer pointers
// (out, in) followed by two 4-byte by_value ints (fmas_per_thread,
// threads_per_block), packed with no padding (mgpusim serializes args with
// binary.Write, which does not insert alignment padding).
type KernelArgs struct {
	Out             driver.Ptr // offset 0
	In              driver.Ptr // offset 8
	FmasPerThread   int32      // offset 16
	ThreadsPerBlock int32      // offset 20
}

// Benchmark defines the fp64_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp64_throughput benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the fp64_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
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
	if b.ThreadsPerBlock <= 0 {
		b.ThreadsPerBlock = defaultThreadsPerBlock
	}
	// Match the host program: round down to a multiple of 4, minimum 4.
	b.FmasPerThread = (b.FmasPerThread / 4) * 4
	if b.FmasPerThread == 0 {
		b.FmasPerThread = 4
	}

	b.NumThreads = b.NumBlocks * b.ThreadsPerBlock
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
	b.In = make([]float64, b.NumThreads*stride)
	for t := 0; t < b.NumThreads; t++ {
		base := t * stride
		b.In[base+0] = seedFor(0)
		b.In[base+1] = seedFor(1)
		b.In[base+2] = seedFor(2)
		b.In[base+3] = seedFor(3)
		b.In[base+4] = fmaMul
		b.In[base+5] = fmaAdd
	}

	inBytes := uint64(len(b.In) * 8)
	outBytes := uint64(b.NumThreads * 8)
	if b.UseUnifiedMemory {
		b.GIn = b.driver.AllocateUnifiedMemory(b.Context, inBytes)
		b.GOut = b.driver.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GIn = b.driver.AllocateMemory(b.Context, inBytes)
		b.GOut = b.driver.AllocateMemory(b.Context, outBytes)
	}

	b.Upload(b.driver, b.Queue, b.GIn, b.In)
	// Initialize the output to zero so a missing store is detectable.
	b.Upload(b.driver, b.Queue, b.GOut, make([]float64, b.NumThreads))
}

func (b *Benchmark) exec() {
	globalX := uint32(b.NumThreads)

	args := KernelArgs{
		Out:             b.GOut,
		In:              b.GIn,
		FmasPerThread:   int32(b.FmasPerThread),
		ThreadsPerBlock: int32(b.ThreadsPerBlock),
	}

	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(b.ThreadsPerBlock), 1, 1},
		&args,
	)
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
func (b *Benchmark) verify() error {
	gpuOut := make([]float64, b.NumThreads)
	if err := benchmarks.Read(b.driver, b.Readbacks[0], gpuOut); err != nil {
		return err
	}

	ref := b.cpuReference()
	denom := math.Abs(ref)
	if denom < 1.0 {
		denom = 1.0
	}

	for gid := 0; gid < b.NumThreads; gid++ {
		got := gpuOut[gid]
		if math.Abs(ref-got)/denom > 1e-3 {
			return fmt.Errorf("At gid %d, expected %.12f, but got %.12f.\n",
				gid, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GOut, 8*(b.NumThreads)))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 2
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
