// Package threemm implements the PolyBench 3MM benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_3mm) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// It computes three chained matrix multiplications:
//
//	E = A * B  (NI×NK · NK×NJ → NI×NJ)
//	F = C * D  (NJ×NM · NM×NL → NJ×NL)
//	G = E * F  (NI×NJ · NJ×NL → NI×NL)
//
// Each thread computes one output element with a simple dot-product loop,
// using a constant 16×16 block. The kernel binary is compiled for gfx942
// only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300A configuration).
package threemm

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const blockSize = 16

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernels.
//
// All three 3mm kernels share an identical signature: three 8-byte
// global_buffer pointers followed by three 4-byte by_value int32 scalars.
// The layout is verified against the compiled kernels' AMDGPU metadata
// (kernarg_segment_size = 36), packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernels read only blockIdx/threadIdx with constant block dims, so no
// hidden ABI arguments are emitted.
type KernelArgs struct {
	In0 driver.Ptr // offset 0  (A / C / E)
	In1 driver.Ptr // offset 8  (B / D / F)
	Out driver.Ptr // offset 16 (E / F / G)
	D0  int32      // offset 24 (NI / NJ / NI)
	D1  int32      // offset 28 (NK / NM / NJ)
	D2  int32      // offset 32 (NJ / NL / NL)
}

// Benchmark defines the 3MM benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco1  *insts.KernelCodeObject
	hsaco2  *insts.KernelCodeObject
	hsaco3  *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// Matrix dimensions. All default to N (set via NewBenchmark caller).
	NI int
	NJ int
	NK int
	NL int
	NM int

	a []float32 // NI×NK
	b []float32 // NK×NJ
	c []float32 // NJ×NM
	d []float32 // NM×NL

	gA driver.Ptr
	gB driver.Ptr
	gC driver.Ptr
	gD driver.Ptr
	gE driver.Ptr
	gF driver.Ptr
	gG driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 3MM benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco1 = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mm3_kernel1")
	b.hsaco2 = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mm3_kernel2")
	b.hsaco3 = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mm3_kernel3")
	if b.hsaco1 == nil || b.hsaco2 == nil || b.hsaco3 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. 3MM uses a single GPU.
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
		log.Panic("the polybench 3mm benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NI <= 0 {
		b.NI = 128
	}
	if b.NJ <= 0 {
		b.NJ = 128
	}
	if b.NK <= 0 {
		b.NK = 128
	}
	if b.NL <= 0 {
		b.NL = 128
	}
	if b.NM <= 0 {
		b.NM = 128
	}

	b.a = make([]float32, b.NI*b.NK)
	b.b = make([]float32, b.NK*b.NJ)
	b.c = make([]float32, b.NJ*b.NM)
	b.d = make([]float32, b.NM*b.NL)

	// Deterministic init reproducible in Verify().
	for i := range b.a {
		b.a[i] = float32(i%100) / 10.0
	}
	for i := range b.b {
		b.b[i] = float32((i*2)%100) / 10.0
	}
	for i := range b.c {
		b.c[i] = float32((i*3)%100) / 10.0
	}
	for i := range b.d {
		b.d[i] = float32((i*4)%100) / 10.0
	}

	alloc := b.driver.AllocateMemory
	if b.useUnifiedMemory {
		alloc = b.driver.AllocateUnifiedMemory
	}

	b.gA = alloc(b.context, uint64(b.NI*b.NK*4))
	b.gB = alloc(b.context, uint64(b.NK*b.NJ*4))
	b.gC = alloc(b.context, uint64(b.NJ*b.NM*4))
	b.gD = alloc(b.context, uint64(b.NM*b.NL*4))
	b.gE = alloc(b.context, uint64(b.NI*b.NJ*4))
	b.gF = alloc(b.context, uint64(b.NJ*b.NL*4))
	b.gG = alloc(b.context, uint64(b.NI*b.NL*4))

	b.driver.MemCopyH2D(b.context, b.gA, b.a)
	b.driver.MemCopyH2D(b.context, b.gB, b.b)
	b.driver.MemCopyH2D(b.context, b.gC, b.c)
	b.driver.MemCopyH2D(b.context, b.gD, b.d)
}

// gridDim returns the smallest multiple of blockSize that is >= n.
func gridDim(n int) uint32 {
	blocks := (n + blockSize - 1) / blockSize
	return uint32(blocks * blockSize)
}

func (b *Benchmark) exec() {
	// Kernel 1: E = A*B, grid over (NJ, NI).
	args1 := KernelArgs{
		In0: b.gA, In1: b.gB, Out: b.gE,
		D0: int32(b.NI), D1: int32(b.NK), D2: int32(b.NJ),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco1,
		[3]uint32{gridDim(b.NJ), gridDim(b.NI), 1},
		[3]uint16{blockSize, blockSize, 1},
		&args1,
	)

	// Kernel 2: F = C*D, grid over (NL, NJ).
	args2 := KernelArgs{
		In0: b.gC, In1: b.gD, Out: b.gF,
		D0: int32(b.NJ), D1: int32(b.NM), D2: int32(b.NL),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco2,
		[3]uint32{gridDim(b.NL), gridDim(b.NJ), 1},
		[3]uint16{blockSize, blockSize, 1},
		&args2,
	)

	// Kernel 3: G = E*F, grid over (NL, NI).
	args3 := KernelArgs{
		In0: b.gE, In1: b.gF, Out: b.gG,
		D0: int32(b.NI), D1: int32(b.NJ), D2: int32(b.NL),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco3,
		[3]uint32{gridDim(b.NL), gridDim(b.NI), 1},
		[3]uint16{blockSize, blockSize, 1},
		&args3,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	gpuG := make([]float32, b.NI*b.NL)
	b.driver.MemCopyD2H(b.context, gpuG, b.gG)

	// E = A * B  (NI×NJ)
	e := make([]float64, b.NI*b.NJ)
	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NJ; j++ {
			var sum float64
			for k := 0; k < b.NK; k++ {
				sum += float64(b.a[i*b.NK+k]) * float64(b.b[k*b.NJ+j])
			}
			e[i*b.NJ+j] = sum
		}
	}

	// F = C * D  (NJ×NL)
	f := make([]float64, b.NJ*b.NL)
	for j := 0; j < b.NJ; j++ {
		for l := 0; l < b.NL; l++ {
			var sum float64
			for m := 0; m < b.NM; m++ {
				sum += float64(b.c[j*b.NM+m]) * float64(b.d[m*b.NL+l])
			}
			f[j*b.NL+l] = sum
		}
	}

	// G = E * F  (NI×NL)
	for i := 0; i < b.NI; i++ {
		for l := 0; l < b.NL; l++ {
			var sum float64
			for j := 0; j < b.NJ; j++ {
				sum += e[i*b.NJ+j] * f[j*b.NL+l]
			}
			ref := sum
			got := float64(gpuG[i*b.NL+l])

			denom := math.Abs(ref)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(ref-got)/denom > 1e-3 {
				log.Fatalf("At (%d,%d), expected %f, but got %f.\n",
					i, l, ref, got)
			}
		}
	}

	log.Printf("Passed!\n")
}
