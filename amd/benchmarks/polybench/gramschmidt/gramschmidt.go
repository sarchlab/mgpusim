// Package gramschmidt implements the PolyBench Gram-Schmidt benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/polybench_gramschmidt) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// It computes the QR factorization of an M x N matrix A via Gram-Schmidt
// orthogonalization, A = Q * R, where Q (M x N) has orthonormal columns and
// R (N x N) is upper triangular. The host iterates over the N columns; for
// each column k it launches three kernels:
//
//	gram_norm_finish : single thread computes nrm = sqrt(sum A[:,k]^2),
//	                   stores R[k,k] = nrm and nrm_buf[0] = nrm
//	gram_normalize   : M threads, Q[:,k] = A[:,k] / nrm
//	gram_project     : one thread per column j > k, removes the Q[:,k]
//	                   component from A[:,j] and records R[k,j]
//
// The original PolyBench kernel computed the column norm with atomicAdd; the
// CDNA3 functional emulator does not implement global atomics, so the norm is
// computed sequentially inside the single-thread gram_norm_finish kernel
// (numerically identical to a serial reduction).
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package gramschmidt

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const blockSize = 256

// NormFinishArgs are the arguments for gram_norm_finish(A, R, nrm_buf, M, N, k).
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 36): three 8-byte global_buffer pointers followed
// by three 4-byte by_value int32 scalars, packed with no padding. The kernel
// uses a constant block size, so no hidden ABI arguments are emitted.
type NormFinishArgs struct {
	A      driver.Ptr // offset 0
	R      driver.Ptr // offset 8
	NrmBuf driver.Ptr // offset 16
	M      int32      // offset 24
	N      int32      // offset 28
	K      int32      // offset 32
}

// NormalizeArgs are the arguments for gram_normalize(A, Q, nrm_buf, M, N, k).
type NormalizeArgs struct {
	A      driver.Ptr // offset 0
	Q      driver.Ptr // offset 8
	NrmBuf driver.Ptr // offset 16
	M      int32      // offset 24
	N      int32      // offset 28
	K      int32      // offset 32
}

// ProjectArgs are the arguments for gram_project(A, Q, R, M, N, k).
type ProjectArgs struct {
	A driver.Ptr // offset 0
	Q driver.Ptr // offset 8
	R driver.Ptr // offset 16
	M int32      // offset 24
	N int32      // offset 28
	K int32      // offset 32
}

// Benchmark defines the Gram-Schmidt benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	gpus    []int

	normFinish *insts.KernelCodeObject
	normalize  *insts.KernelCodeObject
	project    *insts.KernelCodeObject

	Arch arch.Type
	M    int
	N    int

	aInit []float32 // original A, M x N (row-major)
	gA    driver.Ptr
	gQ    driver.Ptr
	gR    driver.Ptr
	gNrm  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Gram-Schmidt benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.normFinish = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "gram_norm_finish")
	if b.normFinish == nil {
		log.Panic("Failed to load gram_norm_finish kernel binary")
	}

	b.normalize = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "gram_normalize")
	if b.normalize == nil {
		log.Panic("Failed to load gram_normalize kernel binary")
	}

	b.project = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "gram_project")
	if b.project == nil {
		log.Panic("Failed to load gram_project kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Gram-Schmidt uses a single GPU.
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
		log.Panic("the polybench gramschmidt benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.M <= 0 {
		b.M = 32
	}
	if b.N <= 0 {
		b.N = 32
	}

	m, n := b.M, b.N

	// Deterministic init in [0,1) using a simple LCG so it is exactly
	// reproducible in Verify(). Values are well-conditioned for QR.
	b.aInit = make([]float32, m*n)
	state := uint32(42)
	for i := 0; i < m*n; i++ {
		state = state*1103515245 + 12345
		b.aInit[i] = float32((state>>16)&0x7fff) / 32768.0
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(m*n*4))
		b.gQ = b.driver.AllocateUnifiedMemory(b.context, uint64(m*n*4))
		b.gR = b.driver.AllocateUnifiedMemory(b.context, uint64(n*n*4))
		b.gNrm = b.driver.AllocateUnifiedMemory(b.context, uint64(4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(m*n*4))
		b.gQ = b.driver.AllocateMemory(b.context, uint64(m*n*4))
		b.gR = b.driver.AllocateMemory(b.context, uint64(n*n*4))
		b.gNrm = b.driver.AllocateMemory(b.context, uint64(4))
	}

	// A starts as aInit; Q and R start at zero.
	b.driver.MemCopyH2D(b.context, b.gA, b.aInit)
	zeroQ := make([]float32, m*n)
	zeroR := make([]float32, n*n)
	b.driver.MemCopyH2D(b.context, b.gQ, zeroQ)
	b.driver.MemCopyH2D(b.context, b.gR, zeroR)
}

func (b *Benchmark) exec() {
	m, n := b.M, b.N
	gridM := uint32((m+blockSize-1)/blockSize) * blockSize

	for k := 0; k < n; k++ {
		// Step 1: nrm = sqrt(sum A[:,k]^2); R[k,k] = nrm; nrm_buf = nrm.
		normFinishArgs := NormFinishArgs{
			A: b.gA, R: b.gR, NrmBuf: b.gNrm,
			M: int32(m), N: int32(n), K: int32(k),
		}
		b.driver.EnqueueLaunchKernel(
			b.queue, b.normFinish,
			[3]uint32{blockSize, 1, 1}, [3]uint16{blockSize, 1, 1},
			&normFinishArgs,
		)

		// Step 2: Q[:,k] = A[:,k] / nrm.
		normalizeArgs := NormalizeArgs{
			A: b.gA, Q: b.gQ, NrmBuf: b.gNrm,
			M: int32(m), N: int32(n), K: int32(k),
		}
		b.driver.EnqueueLaunchKernel(
			b.queue, b.normalize,
			[3]uint32{gridM, 1, 1}, [3]uint16{blockSize, 1, 1},
			&normalizeArgs,
		)

		// Step 3: project remaining columns j > k.
		if k+1 < n {
			remaining := n - k - 1
			gridProj := uint32((remaining+blockSize-1)/blockSize) * blockSize
			projectArgs := ProjectArgs{
				A: b.gA, Q: b.gQ, R: b.gR,
				M: int32(m), N: int32(n), K: int32(k),
			}
			b.driver.EnqueueLaunchKernel(
				b.queue, b.project,
				[3]uint32{gridProj, 1, 1}, [3]uint16{blockSize, 1, 1},
				&projectArgs,
			)
		}

		// Kernels for column k must complete before column k+1 starts
		// (gram_project mutates A in place), so drain between iterations.
		b.driver.DrainCommandQueue(b.queue)
	}
}

// Verify checks the GPU Q and R against a CPU reference that reproduces the
// exact same sequential Gram-Schmidt computation.
func (b *Benchmark) Verify() {
	m, n := b.M, b.N

	gpuQ := make([]float32, m*n)
	gpuR := make([]float32, n*n)
	b.driver.MemCopyD2H(b.context, gpuQ, b.gQ)
	b.driver.MemCopyD2H(b.context, gpuR, b.gR)

	cpuQ, cpuR := b.cpuGramSchmidt()

	tol := 1e-3
	relErr := func(ref, got float32) float64 {
		denom := math.Abs(float64(ref))
		if denom < 1.0 {
			denom = 1.0
		}
		return math.Abs(float64(ref)-float64(got)) / denom
	}

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			if relErr(cpuQ[i*n+j], gpuQ[i*n+j]) > tol {
				log.Fatalf("Q mismatch at (%d,%d): expected %f, got %f",
					i, j, cpuQ[i*n+j], gpuQ[i*n+j])
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if relErr(cpuR[i*n+j], gpuR[i*n+j]) > tol {
				log.Fatalf("R mismatch at (%d,%d): expected %f, got %f",
					i, j, cpuR[i*n+j], gpuR[i*n+j])
			}
		}
	}

	// Sanity: orthonormality of the leading columns (Q^T Q ~ I).
	check := n
	if check > 8 {
		check = 8
	}
	for ci := 0; ci < check; ci++ {
		for cj := 0; cj < check; cj++ {
			var dot float64
			for i := 0; i < m; i++ {
				dot += float64(gpuQ[i*n+ci]) * float64(gpuQ[i*n+cj])
			}
			expected := 0.0
			if ci == cj {
				expected = 1.0
			}
			if math.Abs(dot-expected) > 1e-2 {
				log.Fatalf("orthogonality fail: dot(Q[:,%d],Q[:,%d])=%f, "+
					"expected %f", ci, cj, dot, expected)
			}
		}
	}

	log.Printf("Passed!\n")
}

// cpuGramSchmidt reproduces the GPU computation exactly using float32 math
// in the same sequential order.
func (b *Benchmark) cpuGramSchmidt() (q, r []float32) {
	m, n := b.M, b.N

	a := make([]float32, m*n)
	copy(a, b.aInit)
	q = make([]float32, m*n)
	r = make([]float32, n*n)

	for k := 0; k < n; k++ {
		// norm of column k
		var sum float32
		for i := 0; i < m; i++ {
			v := a[i*n+k]
			sum += v * v
		}
		nrm := float32(math.Sqrt(float64(sum)))
		r[k*n+k] = nrm

		// normalize
		for i := 0; i < m; i++ {
			q[i*n+k] = a[i*n+k] / nrm
		}

		// project remaining columns
		for j := k + 1; j < n; j++ {
			var dot float32
			for i := 0; i < m; i++ {
				dot += q[i*n+k] * a[i*n+j]
			}
			r[k*n+j] = dot
			for i := 0; i < m; i++ {
				a[i*n+j] -= dot * q[i*n+k]
			}
		}
	}

	return q, r
}
