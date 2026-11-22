// Package int32throughput implements the int32_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/int32_throughput) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Each thread runs a long chain of INT32 multiply-add operations
// (a = a*madMul + madAdd) entirely in registers and then writes its accumulated
// result to out[globalThreadId]. The dominant compute is the register-only
// MAD chain (the original throughput microbenchmark), while the per-thread
// store makes the result fully verifiable on the host with int32
// wraparound.
//
// The compiled kernel targets MI300X (gfx942).
package int32throughput

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const (
	defaultThreadsPerBlock        = 256
	madMul                 uint32 = 1664525
	madAdd                 uint32 = 1013904223
)

// KernelArgs is the device ABI for the version 1.1 HIP kernel.
type KernelArgs struct {
	Out           gpuruntime.Ptr
	MadsPerThread int32
	Mul           uint32
	Add           uint32
	_             [4]byte
	Launch        benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the int32_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new int32_throughput benchmark.
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
		cdna3HSACOBytes, "int32_mad_kernel")
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
		log.Panic("the int32_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
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
	if b.ThreadsPerBlock <= 0 {
		b.ThreadsPerBlock = defaultThreadsPerBlock
	}

	// Match the HIP host: round down to a multiple of 4, min 4.
	b.EffectiveMadsPerThread = (b.MadsPerThread / 4) * 4
	if b.EffectiveMadsPerThread == 0 {
		b.EffectiveMadsPerThread = 4
	}

	b.NumThreads = b.NumBlocks * b.ThreadsPerBlock

	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(b.NumThreads*4))
	} else {
		b.GOut = b.rt.AllocateMemory(
			b.Context, uint64(b.NumThreads*4))
	}
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.NumThreads), 1, 1}
	group := [3]uint16{uint16(b.ThreadsPerBlock), 1, 1}
	args := KernelArgs{
		Out: b.GOut, MadsPerThread: int32(b.EffectiveMadsPerThread), Mul: madMul, Add: madAdd,
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// cpuReferenceForThread reproduces, exactly, the value a thread with the
// given threadIdx.x writes. The initial accumulators are
// a0=1+threadIdx.x, a1=a0+111, a2=a0+222, a3=a0+333; all arithmetic is
// unsigned with wraparound.
func (b *Benchmark) cpuReferenceForThread(threadIdxX int) uint32 {
	a0 := uint32(1 + threadIdxX)
	a1 := a0 + 111
	a2 := a0 + 222
	a3 := a0 + 333

	for i := 0; i < b.EffectiveMadsPerThread; i += 4 {
		a0 = a0*madMul + madAdd
		a1 = a1*madMul + madAdd
		a2 = a2*madMul + madAdd
		a3 = a3*madMul + madAdd
	}

	return a0 + a1 + a2 + a3
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	gpuOut := make([]uint32, b.NumThreads)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuOut); err != nil {
		return err
	}

	for tid := 0; tid < b.NumThreads; tid++ {
		threadIdxX := tid % b.ThreadsPerBlock
		ref := b.cpuReferenceForThread(threadIdxX)
		got := gpuOut[tid]

		if ref != got {
			return fmt.Errorf("Verification failed at thread %d: "+
				"expected %d, but got %d.\n", tid, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOut, 4*(b.NumThreads)))
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
