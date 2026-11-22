// Package jacobi2d implements the PolyBench Jacobi-2D stencil benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/polybench_jacobi2d) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// It runs TSTEPS iterations of the 2D Jacobi stencil on an NxN grid:
//
//	B[i][j] = (A[i-1][j] + A[i+1][j] + A[i][j-1] + A[i][j+1] + A[i][j]) * 0.2
//
// with a double buffer swapped between A and B every step. Only interior
// points (i=1..N-2, j=1..N-2) are updated; boundaries remain 0. The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300X configuration).
package jacobi2d

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

// blockDim is the constant work-group size baked into the kernel (BLOCK_DIM).
const blockDim = 16

// KernelArgs is the ABI of both Jacobi dispatches.
type KernelArgs struct {
	A, B   gpuruntime.Ptr
	N      int32
	_      uint32
	Launch benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the Jacobi-2D benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	stencil, copyBack *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Jacobi-2D benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.stencil = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "jacobi2d_kernel")
	b.copyBack = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "jacobi2d_copy_kernel")
	if b.stencil == nil || b.copyBack == nil {
		log.Panic("could not load the PolyBench Jacobi kernels")
	}
}

// SelectGPU selects the GPUs to run on. Jacobi-2D uses a single GPU.
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
		log.Panic("the polybench jacobi2d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 64
	}
	if b.TSteps <= 0 {
		b.TSteps = 10
	}

	n := b.N
	numElem := n * n

	if n < 3 {
		panic("Jacobi requires at least a 3 by 3 grid")
	}
	b.AInit = make([]float32, numElem)
	b.BInit = make([]float32, numElem)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			b.AInit[i*n+j] = (float32(i)*float32(j+2) + 10) / float32(n)
			b.BInit[i*n+j] = (float32(i-4)*float32(j-1) + 11) / float32(n)
		}
	}

	if b.UseUnifiedMemory {
		b.GA = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GA = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
	}

	b.Upload(b.rt, b.Queue, b.GA, b.AInit)
	b.Upload(b.rt, b.Queue, b.GB, b.BInit)
}

func (b *Benchmark) exec() {
	n := b.N
	global := uint32((n - 2 + blockDim - 1) / blockDim * blockDim)
	grid := [3]uint32{global, global, 1}
	group := [3]uint16{blockDim, blockDim, 1}
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	stencil := KernelArgs{A: b.GA, B: b.GB, N: int32(n), Launch: launch}
	copyBack := KernelArgs{A: b.GB, B: b.GA, N: int32(n), Launch: launch}
	for t := 0; t < b.TSteps; t++ {
		b.rt.EnqueueLaunchKernel(b.Queue, b.stencil, grid, group, &stencil)
		b.rt.EnqueueLaunchKernel(b.Queue, b.copyBack, grid, group, &copyBack)
	}
}

func (b *Benchmark) verify() error {
	n := b.N
	a, out := append([]float32(nil), b.AInit...), append([]float32(nil), b.BInit...)
	for t := 0; t < b.TSteps; t++ {
		for i := 1; i < n-1; i++ {
			for j := 1; j < n-1; j++ {
				k := i*n + j
				out[k] = 0.2 * (a[k] + a[k-1] + a[k+1] + a[k+n] + a[k-n])
			}
		}
		for i := 1; i < n-1; i++ {
			from, to := i*n+1, (i+1)*n-1
			copy(a[from:to], out[from:to])
		}
	}

	actual := make([]float32, n*n)
	for grid, expected := range [][]float32{a, out} {
		if err := benchmarks.Read(b.rt, b.Readbacks[grid], actual); err != nil {
			return err
		}
		for i, reference := range expected {
			if actual[i] != reference {
				return fmt.Errorf("grid %d at (%d,%d): got %g, expected %g", grid+1, i/n, i%n, actual[i], reference)
			}
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GA, 4*b.N*b.N))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GB, 4*b.N*b.N))
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
