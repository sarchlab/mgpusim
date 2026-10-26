// Package gaussian implements the Rodinia Gaussian elimination benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/rodinia_gaussian) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// It solves a dense NxN linear system Ax=b. GPU forward elimination is done
// with two kernels (fan1 computes pivot multipliers, fan2 updates the
// submatrix and rhs), iterated over pivot columns t = 0..N-2. CPU
// back-substitution then solves the resulting upper-triangular system, and
// Verify() checks the relative residual ||A*x - b|| / ||b||.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration). The
// kernels use constant block dimensions, so no hidden ABI arguments are
// emitted (kernarg_segment_size = 24 for fan1, 32 for fan2).
package gaussian

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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	fan1Kernel *insts.KernelCodeObject
	fan2Kernel *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Gaussian elimination benchmark.
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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia gaussian benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// initData builds a deterministic, diagonally dominant system that Verify()
// reproduces exactly. A_ii = N + small; off-diagonal in [0, 0.9]; b in [1, 1.9].
func (b *Benchmark) initData() {
	n := b.N
	b.AInit = make([]float32, n*n)
	b.BInit = make([]float32, n)

	// Off-diagonal entries are kept strictly in [0.1, 1.0] (never exactly
	// 0). A pivot column with an exact-zero entry below the diagonal yields
	// a 0/pivot multiplier; that is mathematically just 0, but it exercises
	// the hardware fast-divide's zero-numerator path. Avoiding exact zeros
	// keeps the system well-conditioned and the result faithful.
	for i := 0; i < n*n; i++ {
		b.AInit[i] = float32((i*7+3)%9)/10.0 + 0.1
	}
	for i := 0; i < n; i++ {
		b.AInit[i*n+i] += float32(n) // diagonal dominance: no pivoting needed
	}
	for i := 0; i < n; i++ {
		b.BInit[i] = float32((i*3+1)%9)/10.0 + 1.0
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

	if b.UseUnifiedMemory {
		b.GA = b.driver.AllocateUnifiedMemory(b.Context, bytesA)
		b.GB = b.driver.AllocateUnifiedMemory(b.Context, bytesB)
		b.GM = b.driver.AllocateUnifiedMemory(b.Context, bytesA)
	} else {
		b.GA = b.driver.AllocateMemory(b.Context, bytesA)
		b.GB = b.driver.AllocateMemory(b.Context, bytesB)
		b.GM = b.driver.AllocateMemory(b.Context, bytesA)
	}

	b.Upload(b.driver, b.Queue, b.GA, b.AInit)
	b.Upload(b.driver, b.Queue, b.GB, b.BInit)

	// Zero-initialize the multiplier matrix.
	mZero := make([]float32, n*n)
	b.Upload(b.driver, b.Queue, b.GM, mZero)
}

func (b *Benchmark) exec() {
	n := b.N

	for t := 0; t < n-1; t++ {
		remaining := n - t - 1

		// fan1: 1D grid, BLOCK1D threads/block.
		grid1 := uint32((remaining + block1D - 1) / block1D)
		fan1Args := Fan1KernelArgs{
			M:    b.GM,
			A:    b.GA,
			Size: int32(n),
			T:    int32(t),
		}
		b.driver.EnqueueLaunchKernel(
			b.Queue,
			b.fan1Kernel,
			[3]uint32{grid1 * block1D, 1, 1},
			[3]uint16{block1D, 1, 1},
			&fan1Args,
		)

		// fan2: 2D grid, BLOCK2D x BLOCK2D threads/block.
		grid2 := uint32((remaining + block2D - 1) / block2D)
		fan2Args := Fan2KernelArgs{
			M:    b.GM,
			A:    b.GA,
			B:    b.GB,
			Size: int32(n),
			T:    int32(t),
		}
		b.driver.EnqueueLaunchKernel(
			b.Queue,
			b.fan2Kernel,
			[3]uint32{grid2 * block2D, grid2 * block2D, 1},
			[3]uint16{block2D, block2D, 1},
			&fan2Args,
		)
	}
}

// Verify checks the GPU forward-elimination result by performing CPU
// back-substitution on the factored system and comparing the relative
// residual ||A*x - b|| / ||b|| of the original system against a tolerance.
func (b *Benchmark) verify() error {
	n := b.N

	a := make([]float32, n*n)
	bb := make([]float32, n)
	if err := benchmarks.Read(b.driver, b.Readbacks[0], a); err != nil {
		return err
	}
	if err := benchmarks.Read(b.driver, b.Readbacks[1], bb); err != nil {
		return err
	}

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
		res := -float64(b.BInit[i])
		for j := 0; j < n; j++ {
			res += float64(b.AInit[i*n+j]) * x[j]
		}
		normRes += res * res
		normB += float64(b.BInit[i]) * float64(b.BInit[i])
	}
	relErr := math.Sqrt(normRes / (normB + 1e-30))

	if math.IsNaN(relErr) || math.IsInf(relErr, 0) || relErr >= 1e-3 {
		return fmt.Errorf("Verification failed: relative residual = %.3e "+
			"(tolerance 1e-3)\n", relErr)
	}

	log.Printf("Verification: rel_err = %.3e\n", relErr)
	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GA, 4*((b.N)*(b.N))))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GB, 4*(b.N)))
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
