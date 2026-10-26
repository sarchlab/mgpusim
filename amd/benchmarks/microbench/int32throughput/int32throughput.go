// Package int32throughput implements the int32_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/int32_throughput) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Each thread runs a long chain of INT32 multiply-add operations
// (a = a*mul + add) entirely in registers and then writes its accumulated
// result to out[globalThreadId]. The dominant compute is the register-only
// MAD chain (the original throughput microbenchmark), while the per-thread
// store makes the result fully verifiable on the host with int32
// wraparound.
//
// The block size is passed as an explicit threads_per_block kernel argument
// (blockDim.x reads back as 0 in this model), so the calibration can sweep it
// and the global thread id stays correct. The kernel binary is compiled for
// gfx942 only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300X configuration).
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

// defaultThreadsPerBlock is the work-group size used when ThreadsPerBlock is
// unset. The block size is a runtime kernel argument (passed explicitly
// because blockDim.x reads back as 0 in this model), so the calibration can
// sweep it.
const defaultThreadsPerBlock = 256

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 16): one 8-byte global_buffer pointer
// followed by two 4-byte by_value int32s (mads_per_thread,
// threads_per_block), packed with no padding (mgpusim serializes args with
// binary.Write, which does not insert alignment padding).
type KernelArgs struct {
	Out             gpuruntime.Ptr // offset 0
	MadsPerThread   int32          // offset 8
	ThreadsPerBlock int32          // offset 12
}

// Benchmark defines the int32_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new int32_throughput benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

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
	globalX := uint32(b.NumThreads)

	args := KernelArgs{
		Out:             b.GOut,
		MadsPerThread:   int32(b.EffectiveMadsPerThread),
		ThreadsPerBlock: int32(b.ThreadsPerBlock),
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(b.ThreadsPerBlock), 1, 1},
		&args,
	)
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

	for i := 0; i < b.EffectiveMadsPerThread; i += 4 {
		a0 = a0*mul + add
		a1 = a1*mul + add
		a2 = a2*mul + add
		a3 = a3*mul + add
	}

	return a0 + a1 + a2 + a3
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	gpuOut := make([]int32, b.NumThreads)
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
