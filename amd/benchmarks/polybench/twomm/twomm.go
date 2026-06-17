// Package twomm implements the PolyBench 2MM benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_2mm) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// 2MM computes two chained matrix multiplications:
//
//	D = alpha*A*B + beta*D
//	E = alpha*C*D + beta*E
//
// for NxN square matrices. Both multiplications use the same shared-memory
// tiled GEMM kernel (mm_kernel, TILE_SIZE = 16), launched twice. The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300A configuration).
package twomm

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

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) mm_kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 36): three 8-byte global_buffer
// pointers followed by three 4-byte by_value scalars, packed with no
// padding (mgpusim serializes args with binary.Write, which does not
// insert alignment padding). The kernel reads only blockIdx/threadIdx,
// so no hidden ABI arguments are emitted.
type KernelArgs struct {
	P     driver.Ptr // offset 0
	Q     driver.Ptr // offset 8
	Out   driver.Ptr // offset 16
	N     int32      // offset 24
	Alpha float32    // offset 28
	Beta  float32    // offset 32
}

// Benchmark defines the 2MM benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	N    int

	a     []float32
	b     []float32
	c     []float32
	dInit []float32
	eInit []float32

	gA driver.Ptr
	gB driver.Ptr
	gC driver.Ptr
	gD driver.Ptr
	gE driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 2MM benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mm_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. 2MM uses a single GPU.
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
		log.Panic("the polybench 2mm benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 128
	}

	n := b.N
	numElem := n * n

	b.a = make([]float32, numElem)
	b.b = make([]float32, numElem)
	b.c = make([]float32, numElem)
	b.dInit = make([]float32, numElem)
	b.eInit = make([]float32, numElem)
	for i := 0; i < numElem; i++ {
		b.a[i] = float32(i%100) / 100.0
		b.b[i] = float32((i*2)%100) / 100.0
		b.c[i] = float32((i*3)%100) / 100.0
		b.dInit[i] = float32((i*4)%100) / 100.0
		b.eInit[i] = float32((i*5)%100) / 100.0
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gB = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gC = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gD = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gE = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gB = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gC = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gD = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gE = b.driver.AllocateMemory(b.context, uint64(numElem*4))
	}

	b.driver.MemCopyH2D(b.context, b.gA, b.a)
	b.driver.MemCopyH2D(b.context, b.gB, b.b)
	b.driver.MemCopyH2D(b.context, b.gC, b.c)
	b.driver.MemCopyH2D(b.context, b.gD, b.dInit)
	b.driver.MemCopyH2D(b.context, b.gE, b.eInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + tileSize - 1) / tileSize)
	globalX := gridDim * tileSize
	globalY := gridDim * tileSize

	// First multiplication: D = alpha*A*B + beta*D
	args1 := KernelArgs{
		P:     b.gA,
		Q:     b.gB,
		Out:   b.gD,
		N:     int32(n),
		Alpha: alpha,
		Beta:  beta,
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, globalY, 1},
		[3]uint16{tileSize, tileSize, 1},
		&args1,
	)

	// Drain before the second launch: E = alpha*C*D + beta*E depends on the
	// full D matrix produced above, so the first kernel's writes to D must be
	// globally visible before the second kernel reads them.
	b.driver.DrainCommandQueue(b.queue)

	// Second multiplication: E = alpha*C*D + beta*E
	args2 := KernelArgs{
		P:     b.gC,
		Q:     b.gD,
		Out:   b.gE,
		N:     int32(n),
		Alpha: alpha,
		Beta:  beta,
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, globalY, 1},
		[3]uint16{tileSize, tileSize, 1},
		&args2,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.N

	// CPU reference for D = alpha*A*B + beta*D.
	//
	// The GPU stores the intermediate matrix D back to global memory as
	// float32 before the second multiplication reads it, so the reference
	// rounds D to float32 too — otherwise the float64 intermediate would
	// drift from the GPU result through the chained product.
	refD := make([]float32, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < n; k++ {
				sum += float64(b.a[i*n+k]) * float64(b.b[k*n+j])
			}
			refD[i*n+j] = float32(
				float64(alpha)*sum + float64(beta)*float64(b.dInit[i*n+j]))
		}
	}

	// CPU reference for E = alpha*C*D + beta*E (using the new D)
	refE := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < n; k++ {
				sum += float64(b.c[i*n+k]) * float64(refD[k*n+j])
			}
			refE[i*n+j] = float64(alpha)*sum + float64(beta)*float64(b.eInit[i*n+j])
		}
	}

	gpuE := make([]float32, n*n)
	b.driver.MemCopyD2H(b.context, gpuE, b.gE)

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			ref := refE[i*n+j]
			got := float64(gpuE[i*n+j])

			denom := math.Abs(ref)
			if denom < 1.0 {
				denom = 1.0
			}
			// Tolerance is looser than a single GEMM (1e-3): 2MM chains two
			// single-precision matmuls, so float32 reduction rounding
			// compounds through the product. A genuinely wrong result would
			// differ by orders of magnitude, not ~1%.
			if math.Abs(ref-got)/denom > 1e-2 {
				log.Fatalf("At (%d,%d), expected %f, but got %f.\n",
					i, j, ref, got)
			}
		}
	}

	log.Printf("Passed!\n")
}
