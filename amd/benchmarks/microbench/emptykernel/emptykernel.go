// Package emptykernel implements the empty_kernel launch-overhead microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/empty_kernel) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// The kernel does no work, so the simulated kernel time for a single launch is the
// model's pure launch / dispatch / grid-scheduling / completion overhead --
// directly comparable to the real benchmark's launch_sync_us (the round-trip
// launch+synchronize cost per launch). The kernel binary is compiled for gfx942
// only (see native/), so the benchmark must be run with `-arch cdna3`.
package emptykernel

import (
	// embed hsaco files
	_ "embed"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// emptyArgs is the kernel argument struct: one unused pointer, present only so
// the kernarg segment is non-zero (Runtime cannot allocate 0 bytes). The
// kernel never dereferences it.
type emptyArgs struct {
	Out gpuruntime.Ptr // offset 0
}

// Benchmark defines the empty_kernel benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new empty_kernel benchmark.
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
		cdna3HSACOBytes, "empty_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. empty_kernel uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the empty_kernel benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
}

func (b *Benchmark) exec() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 1
	}
	if b.BlockSize <= 0 {
		b.BlockSize = 256
	}

	// A tiny unused buffer just to give the kernel a valid (non-zero) kernarg.
	var out gpuruntime.Ptr
	if b.UseUnifiedMemory {
		out = b.rt.AllocateUnifiedMemory(b.Context, 4)
	} else {
		out = b.rt.AllocateMemory(b.Context, 4)
	}

	globalX := uint32(b.NumBlocks * b.BlockSize)
	args := emptyArgs{Out: out}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(b.BlockSize), 1, 1},
		&args,
	)
}

// Verify is a no-op: the empty kernel produces no output to check.
func (b *Benchmark) verify() error {
	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.setup()
		b.exec()
		b.Phase = 1
	case 1:
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}
