// Package jacobi2d implements the PolyBench Jacobi-2D stencil benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/polybench_jacobi2d) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// It runs TSTEPS iterations of the 2D Jacobi stencil on an NxN grid:
//
//	B[i][j] = (A[i-1][j] + A[i+1][j] + A[i][j-1] + A[i][j+1] + A[i][j]) * 0.2
//
// with a double buffer swapped between A and B every step. Only interior
// points (i=1..N-2, j=1..N-2) are updated; boundaries remain 0. The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300A configuration).
package jacobi2d

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockDim is the constant work-group size baked into the kernel (BLOCK_DIM).
const blockDim = 16

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 20): two 8-byte global_buffer pointers
// followed by one 4-byte by_value scalar, packed with no padding (mgpusim
// serializes args with binary.Write, which does not insert alignment
// padding). The kernel uses a constant BLOCK_DIM and reads only
// blockIdx/threadIdx, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	A driver.Ptr // offset 0
	B driver.Ptr // offset 8
	N int32      // offset 16
}

// Benchmark defines the Jacobi-2D benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch   arch.Type
	N      int
	TSteps int

	aInit []float32
	gA    driver.Ptr
	gB    driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Jacobi-2D benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "jacobi2d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Jacobi-2D uses a single GPU.
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
		log.Panic("the polybench jacobi2d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
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

	// Deterministic interior init (boundaries stay 0). Reproduced exactly in
	// Verify().
	b.aInit = make([]float32, numElem)
	for i := 1; i < n-1; i++ {
		for j := 1; j < n-1; j++ {
			b.aInit[i*n+j] = float32((i*7+j*13)%100) / 10.0
		}
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gB = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gB = b.driver.AllocateMemory(b.context, uint64(numElem*4))
	}

	// A holds the initial grid; B starts zeroed.
	b.driver.MemCopyH2D(b.context, b.gA, b.aInit)
	zeros := make([]float32, numElem)
	b.driver.MemCopyH2D(b.context, b.gB, zeros)
}

func (b *Benchmark) exec() {
	n := b.N
	// Grid covers the (N-2)x(N-2) interior, rounded up to whole blocks.
	gridDimX := uint32((n - 2 + blockDim - 1) / blockDim)
	gridDimY := uint32((n - 2 + blockDim - 1) / blockDim)
	globalX := gridDimX * blockDim
	globalY := gridDimY * blockDim

	src, dst := b.gA, b.gB
	for t := 0; t < b.TSteps; t++ {
		args := KernelArgs{
			A: src,
			B: dst,
			N: int32(n),
		}

		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockDim, blockDim, 1},
			&args,
		)
		b.driver.DrainCommandQueue(b.queue)

		src, dst = dst, src
	}

	// After the loop, src points at the buffer holding the final result.
	// Store it in gA so Verify reads a single, well-defined location.
	b.gA = src
	b.gB = dst
}

// cpuStep computes one Jacobi-2D step on the CPU (interior only).
func cpuStep(a, out []float32, n int) {
	for i := 1; i < n-1; i++ {
		for j := 1; j < n-1; j++ {
			out[i*n+j] = (a[(i-1)*n+j] + a[(i+1)*n+j] +
				a[i*n+(j-1)] + a[i*n+(j+1)] +
				a[i*n+j]) * 0.2
		}
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.N
	numElem := n * n

	gpuResult := make([]float32, numElem)
	b.driver.MemCopyD2H(b.context, gpuResult, b.gA)

	// CPU reference: same double-buffered iteration the GPU ran.
	ca := make([]float32, numElem)
	cb := make([]float32, numElem)
	copy(ca, b.aInit)

	src, dst := ca, cb
	for t := 0; t < b.TSteps; t++ {
		cpuStep(src, dst, n)
		src, dst = dst, src
	}
	// src now holds the CPU result.

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			ref := float64(src[i*n+j])
			got := float64(gpuResult[i*n+j])

			denom := math.Abs(ref)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(ref-got)/denom > 1e-3 {
				log.Fatalf("At (%d,%d), expected %f, but got %f.\n",
					i, j, ref, got)
			}
		}
	}

	log.Printf("Passed!\n")
}
