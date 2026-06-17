// Package syr2k implements the PolyBench SYR2K benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_syr2k) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// It computes the symmetric rank-2k update
//
//	C = alpha*A*B^T + alpha*B*A^T + beta*C
//
// where A and B are NxM matrices and C is an NxN matrix, using a
// shared-memory tiled kernel (TILE_SIZE = 16). The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run
// with `-arch cdna3` (the MI300A configuration).
package syr2k

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const tileSize = 16

const (
	alpha float32 = 1.5
	beta  float32 = 1.2
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 40): three 8-byte global_buffer
// pointers (A, B, C) followed by four 4-byte by_value scalars
// (N, M, alpha, beta), packed with no padding (mgpusim serializes args
// with binary.Write, which does not insert alignment padding). The kernel
// reads only blockIdx/threadIdx with a constant block size, so no hidden
// ABI arguments are emitted.
type KernelArgs struct {
	A     driver.Ptr // offset 0
	B     driver.Ptr // offset 8
	C     driver.Ptr // offset 16
	N     int32      // offset 24
	M     int32      // offset 28
	Alpha float32    // offset 32
	Beta  float32    // offset 36
}

// Benchmark defines the SYR2K benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	N    int // rows/cols of C, rows of A and B
	M    int // columns of A and B (the contraction dimension)

	a     []float32
	b     []float32
	cInit []float32
	gA    driver.Ptr
	gB    driver.Ptr
	gC    driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new SYR2K benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "polybench_syr2k_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. SYR2K uses a single GPU.
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
		log.Panic("the polybench syr2k benchmark ships only a gfx942 " +
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
	if b.M <= 0 {
		b.M = 64
	}

	n := b.N
	m := b.M

	b.a = make([]float32, n*m)
	b.b = make([]float32, n*m)
	b.cInit = make([]float32, n*n)
	for i := 0; i < n*m; i++ {
		b.a[i] = float32(i%100) / 100.0
		b.b[i] = float32((i*2)%100) / 100.0
	}
	for i := 0; i < n*n; i++ {
		b.cInit[i] = float32((i*3)%100) / 100.0
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(n*m*4))
		b.gB = b.driver.AllocateUnifiedMemory(b.context, uint64(n*m*4))
		b.gC = b.driver.AllocateUnifiedMemory(b.context, uint64(n*n*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(n*m*4))
		b.gB = b.driver.AllocateMemory(b.context, uint64(n*m*4))
		b.gC = b.driver.AllocateMemory(b.context, uint64(n*n*4))
	}

	b.driver.MemCopyH2D(b.context, b.gA, b.a)
	b.driver.MemCopyH2D(b.context, b.gB, b.b)
	b.driver.MemCopyH2D(b.context, b.gC, b.cInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + tileSize - 1) / tileSize)
	global := gridDim * tileSize

	args := KernelArgs{
		A:     b.gA,
		B:     b.gB,
		C:     b.gC,
		N:     int32(n),
		M:     int32(b.M),
		Alpha: alpha,
		Beta:  beta,
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{global, global, 1},
		[3]uint16{tileSize, tileSize, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation that
// mirrors the kernel exactly:
//
//	C[i][j] = alpha * sum_k( A[i][k]*B[j][k] + B[i][k]*A[j][k] )
//	        + beta  * C_init[i][j]
func (b *Benchmark) Verify() {
	n := b.N
	m := b.M
	gpuC := make([]float32, n*n)
	b.driver.MemCopyD2H(b.context, gpuC, b.gC)

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < m; k++ {
				sum += float64(b.a[i*m+k])*float64(b.b[j*m+k]) +
					float64(b.b[i*m+k])*float64(b.a[j*m+k])
			}
			ref := float64(alpha)*sum + float64(beta)*float64(b.cInit[i*n+j])
			got := float64(gpuC[i*n+j])

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
