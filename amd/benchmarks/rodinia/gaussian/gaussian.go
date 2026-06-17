// Package gaussian implements the Rodinia Gaussian elimination benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/rodinia_gaussian) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// It solves a dense NxN linear system Ax=b. GPU forward elimination is done
// with two kernels (fan1 computes pivot multipliers, fan2 updates the
// submatrix and rhs), iterated over pivot columns t = 0..N-2. CPU
// back-substitution then solves the resulting upper-triangular system, and
// Verify() checks the relative residual ||A*x - b|| / ||b||.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration). The
// kernels use constant block dimensions, so no hidden ABI arguments are
// emitted (kernarg_segment_size = 24 for fan1, 32 for fan2).
package gaussian

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const (
	block1D = 256
	block2D = 16
)

// Fan1KernelArgs defines the kernel arguments for fan1 (gfx942 / CDNA3).
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 24): two 8-byte global_buffer pointers followed by
// two 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel uses a constant block dimension, so no hidden ABI args are emitted.
type Fan1KernelArgs struct {
	M    driver.Ptr // offset 0
	A    driver.Ptr // offset 8
	Size int32      // offset 16
	T    int32      // offset 20
}

// Fan2KernelArgs defines the kernel arguments for fan2 (gfx942 / CDNA3).
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 32): three 8-byte global_buffer pointers followed
// by two 4-byte by_value scalars, packed with no padding. No hidden ABI args.
type Fan2KernelArgs struct {
	M    driver.Ptr // offset 0
	A    driver.Ptr // offset 8
	B    driver.Ptr // offset 16
	Size int32      // offset 24
	T    int32      // offset 28
}

// Benchmark defines the Gaussian elimination benchmark.
type Benchmark struct {
	driver     *driver.Driver
	context    *driver.Context
	queue      *driver.CommandQueue
	fan1Kernel *insts.KernelCodeObject
	fan2Kernel *insts.KernelCodeObject
	gpus       []int

	Arch arch.Type
	N    int

	aInit []float32 // original A (row-major NxN)
	bInit []float32 // original b (N)

	gA driver.Ptr
	gB driver.Ptr
	gM driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Gaussian elimination benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.fan1Kernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "fan1")
	if b.fan1Kernel == nil {
		log.Panic("Failed to load kernel binary fan1")
	}

	b.fan2Kernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "fan2")
	if b.fan2Kernel == nil {
		log.Panic("Failed to load kernel binary fan2")
	}
}

// SelectGPU selects the GPUs to run on. Gaussian uses a single GPU.
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
		log.Panic("the rodinia gaussian benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// initData builds a deterministic, diagonally dominant system that Verify()
// reproduces exactly. A_ii = N + small; off-diagonal in [0, 0.9]; b in [1, 1.9].
func (b *Benchmark) initData() {
	n := b.N
	b.aInit = make([]float32, n*n)
	b.bInit = make([]float32, n)

	// Off-diagonal entries are kept strictly in [0.1, 1.0] (never exactly
	// 0). A pivot column with an exact-zero entry below the diagonal yields
	// a 0/pivot multiplier; that is mathematically just 0, but it exercises
	// the hardware fast-divide's zero-numerator path. Avoiding exact zeros
	// keeps the system well-conditioned and the result faithful.
	for i := 0; i < n*n; i++ {
		b.aInit[i] = float32((i*7+3)%9)/10.0 + 0.1
	}
	for i := 0; i < n; i++ {
		b.aInit[i*n+i] += float32(n) // diagonal dominance: no pivoting needed
	}
	for i := 0; i < n; i++ {
		b.bInit[i] = float32((i*3+1)%9)/10.0 + 1.0
	}
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 16
	}

	b.initData()

	n := b.N
	bytesA := uint64(n * n * 4)
	bytesB := uint64(n * 4)

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, bytesA)
		b.gB = b.driver.AllocateUnifiedMemory(b.context, bytesB)
		b.gM = b.driver.AllocateUnifiedMemory(b.context, bytesA)
	} else {
		b.gA = b.driver.AllocateMemory(b.context, bytesA)
		b.gB = b.driver.AllocateMemory(b.context, bytesB)
		b.gM = b.driver.AllocateMemory(b.context, bytesA)
	}

	b.driver.MemCopyH2D(b.context, b.gA, b.aInit)
	b.driver.MemCopyH2D(b.context, b.gB, b.bInit)

	// Zero-initialize the multiplier matrix.
	mZero := make([]float32, n*n)
	b.driver.MemCopyH2D(b.context, b.gM, mZero)
}

func (b *Benchmark) exec() {
	n := b.N

	for t := 0; t < n-1; t++ {
		remaining := n - t - 1

		// fan1: 1D grid, BLOCK1D threads/block.
		grid1 := uint32((remaining + block1D - 1) / block1D)
		fan1Args := Fan1KernelArgs{
			M:    b.gM,
			A:    b.gA,
			Size: int32(n),
			T:    int32(t),
		}
		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.fan1Kernel,
			[3]uint32{grid1 * block1D, 1, 1},
			[3]uint16{block1D, 1, 1},
			&fan1Args,
		)

		// fan2: 2D grid, BLOCK2D x BLOCK2D threads/block.
		grid2 := uint32((remaining + block2D - 1) / block2D)
		fan2Args := Fan2KernelArgs{
			M:    b.gM,
			A:    b.gA,
			B:    b.gB,
			Size: int32(n),
			T:    int32(t),
		}
		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.fan2Kernel,
			[3]uint32{grid2 * block2D, grid2 * block2D, 1},
			[3]uint16{block2D, block2D, 1},
			&fan2Args,
		)
	}

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU forward-elimination result by performing CPU
// back-substitution on the factored system and comparing the relative
// residual ||A*x - b|| / ||b|| of the original system against a tolerance.
func (b *Benchmark) Verify() {
	n := b.N

	a := make([]float32, n*n)
	bb := make([]float32, n)
	b.driver.MemCopyD2H(b.context, a, b.gA)
	b.driver.MemCopyD2H(b.context, bb, b.gB)

	// CPU back-substitution: solve upper-triangular U x = b.
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := float64(bb[i])
		for j := i + 1; j < n; j++ {
			sum -= float64(a[i*n+j]) * x[j]
		}
		x[i] = sum / float64(a[i*n+i])
	}

	// Relative residual of the ORIGINAL system using the original A and b.
	var normRes, normB float64
	for i := 0; i < n; i++ {
		res := -float64(b.bInit[i])
		for j := 0; j < n; j++ {
			res += float64(b.aInit[i*n+j]) * x[j]
		}
		normRes += res * res
		normB += float64(b.bInit[i]) * float64(b.bInit[i])
	}
	relErr := math.Sqrt(normRes / (normB + 1e-30))

	if math.IsNaN(relErr) || math.IsInf(relErr, 0) || relErr >= 1e-3 {
		log.Fatalf("Verification failed: relative residual = %.3e "+
			"(tolerance 1e-3)\n", relErr)
	}

	log.Printf("Verification: rel_err = %.3e\n", relErr)
	log.Printf("Passed!\n")
}
