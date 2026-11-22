// Package fp32throughput implements the fp32_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/fp32_throughput) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Each work-item runs a chain of fused multiply-add (FMA) operations on
// register-resident floats using four independent accumulators. The kernel
// stores one checksum per work-item, which the CPU verifier reproduces. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X configuration).
package fp32throughput

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/bitops"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// FMA multiplier / addend constants, kept identical to the HIP kernel so the
// CPU reference in Verify() matches the GPU result bit-for-bit in spirit.
const (
	fmaMul float32 = 1.0000001
	fmaAdd float32 = 0.0000001
)

// KernelArgs is the device ABI for the version 1.1 HIP kernel.
type KernelArgs struct {
	Out           gpuruntime.Ptr
	FmasPerThread int32
	_             [4]byte
	Launch        benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the fp32_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp32_throughput benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the fp32_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
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

	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GOut = b.rt.AllocateMemory(b.Context, outBytes)
	}

	// Initialize the output buffer to a known value.
	b.Upload(b.rt, b.Queue, b.GOut, make([]float32, numElem))
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.numThreads()), 1, 1}
	group := [3]uint16{uint16(b.threadsPerBlock()), 1, 1}
	args := KernelArgs{
		Out: b.GOut, FmasPerThread: b.fmasPerThread(),
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// Verify checks the GPU result against a CPU reference computation.
//
// Each thread writes out[tid] = a0+a1+a2+a3, where the four accumulators start
// at (1 + threadIdx.x*0.001) plus 0/0.1/0.2/0.3 and each is iterated
// fmas_per_thread/4 times through a = a*mul + add. Since threadIdx.x is the
// lane index within a block, the reference depends only on threadIdx.x and is
// reproduced exactly here using float32 arithmetic.
func (b *Benchmark) verify() error {
	numElem := b.numThreads()
	gpu := make([]float32, numElem)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpu); err != nil {
		return err
	}

	iters := int(b.fmasPerThread()) / 4
	tpb := b.threadsPerBlock()

	// The result for a thread depends only on threadIdx.x (== tid % block size),
	// so precompute one reference value per lane.
	ref := make([]float32, tpb)
	for lane := 0; lane < tpb; lane++ {
		a0 := bitops.FMAF32(float32(lane), 0.001, 1)
		a1 := a0 + 0.1
		a2 := a0 + 0.2
		a3 := a0 + 0.3

		for i := 0; i < iters; i++ {
			a0 = bitops.FMAF32(a0, fmaMul, fmaAdd)
			a1 = bitops.FMAF32(a1, fmaMul, fmaAdd)
			a2 = bitops.FMAF32(a2, fmaMul, fmaAdd)
			a3 = bitops.FMAF32(a3, fmaMul, fmaAdd)
		}
		ref[lane] = float32(float32(a0+a1)+a2) + a3
	}

	for tid := 0; tid < numElem; tid++ {
		want, got := ref[tid%tpb], gpu[tid]
		if math.Float32bits(got) != math.Float32bits(want) {
			return fmt.Errorf("At thread %d, expected %f, but got %f.\n",
				tid, want, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOut, 4*(b.numThreads())))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
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
