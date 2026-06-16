// Package babelstream implements the BabelStream benchmark, ported from
// sarchlab/gpu_benchmarks (tier4/babelstream) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// BabelStream measures memory bandwidth via four elementwise vector
// operations over float arrays of length N:
//
//	copy:   c[i] = a[i]
//	scale:  b[i] = s * c[i]
//	add:    c[i] = a[i] + b[i]
//	triad:  a[i] = b[i] + s * c[i]
//
// The four operations are run once each, in this order, exactly as the host
// loop in the original benchmark drives them. Verify() reproduces the same
// sequence on the CPU.
//
// The kernels are compiled for gfx942 only (see native/) with a constant
// block size of 256, so they emit no hidden ABI arguments. The benchmark
// must be run with `-arch cdna3` (the MI300A configuration).
package babelstream

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the constant work-group size baked into the kernels (see
// native/babelstream.cpp). It must match the BLOCK_SIZE literal there.
const blockSize = 256

// CopyKernelArgs matches copy_kernel (kernarg_segment_size = 20):
// a(ptr,0), c(ptr,8), n(i32,16).
type CopyKernelArgs struct {
	A driver.Ptr // offset 0
	C driver.Ptr // offset 8
	N int32      // offset 16
}

// ScaleKernelArgs matches scale_kernel (kernarg_segment_size = 24):
// c(ptr,0), b(ptr,8), s(f32,16), n(i32,20).
type ScaleKernelArgs struct {
	C driver.Ptr // offset 0
	B driver.Ptr // offset 8
	S float32    // offset 16
	N int32      // offset 20
}

// AddKernelArgs matches add_kernel (kernarg_segment_size = 28):
// a(ptr,0), b(ptr,8), c(ptr,16), n(i32,24).
type AddKernelArgs struct {
	A driver.Ptr // offset 0
	B driver.Ptr // offset 8
	C driver.Ptr // offset 16
	N int32      // offset 24
}

// TriadKernelArgs matches triad_kernel (kernarg_segment_size = 32):
// b(ptr,0), c(ptr,8), a(ptr,16), s(f32,24), n(i32,28).
type TriadKernelArgs struct {
	B driver.Ptr // offset 0
	C driver.Ptr // offset 8
	A driver.Ptr // offset 16
	S float32    // offset 24
	N int32      // offset 28
}

// Benchmark defines the BabelStream benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	gpus    []int

	copyKernel  *insts.KernelCodeObject
	scaleKernel *insts.KernelCodeObject
	addKernel   *insts.KernelCodeObject
	triadKernel *insts.KernelCodeObject

	Arch   arch.Type
	N      int
	Scalar float32

	a []float32
	b []float32
	c []float32

	gA driver.Ptr
	gB driver.Ptr
	gC driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new BabelStream benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.copyKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "copy_kernel")
	b.scaleKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "scale_kernel")
	b.addKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "add_kernel")
	b.triadKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "triad_kernel")

	if b.copyKernel == nil || b.scaleKernel == nil ||
		b.addKernel == nil || b.triadKernel == nil {
		log.Panic("Failed to load one or more kernel binaries")
	}
}

// SelectGPU selects the GPUs to run on. BabelStream uses a single GPU.
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
		log.Panic("the babelstream benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 4096
	}
	if b.Scalar == 0 {
		b.Scalar = 2.0
	}

	n := b.N

	b.a = make([]float32, n)
	b.b = make([]float32, n)
	b.c = make([]float32, n)
	// Deterministic, varied host init so the verification is meaningful.
	for i := 0; i < n; i++ {
		b.a[i] = float32(i%100) / 100.0
		b.b[i] = float32((i*2)%100) / 100.0
		b.c[i] = 0.0
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.gB = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.gC = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.gB = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.gC = b.driver.AllocateMemory(b.context, uint64(n*4))
	}

	b.driver.MemCopyH2D(b.context, b.gA, b.a)
	b.driver.MemCopyH2D(b.context, b.gB, b.b)
	b.driver.MemCopyH2D(b.context, b.gC, b.c)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize

	// copy: c[i] = a[i]
	copyArgs := CopyKernelArgs{A: b.gA, C: b.gC, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.copyKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&copyArgs,
	)

	// scale: b[i] = s * c[i]
	scaleArgs := ScaleKernelArgs{C: b.gC, B: b.gB, S: b.Scalar, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.scaleKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&scaleArgs,
	)

	// add: c[i] = a[i] + b[i]
	addArgs := AddKernelArgs{A: b.gA, B: b.gB, C: b.gC, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.addKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&addArgs,
	)

	// triad: a[i] = b[i] + s * c[i]
	triadArgs := TriadKernelArgs{B: b.gB, C: b.gC, A: b.gA, S: b.Scalar, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.triadKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&triadArgs,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the same copy -> scale -> add -> triad sequence.
func (b *Benchmark) Verify() {
	n := b.N
	s := float64(b.Scalar)

	// CPU reference, mirroring exec() order on a copy of the init data.
	refA := make([]float64, n)
	refB := make([]float64, n)
	refC := make([]float64, n)
	for i := 0; i < n; i++ {
		refA[i] = float64(b.a[i])
		refB[i] = float64(b.b[i])
		refC[i] = 0.0
	}
	for i := 0; i < n; i++ {
		refC[i] = refA[i]             // copy
		refB[i] = s * refC[i]         // scale
		refC[i] = refA[i] + refB[i]   // add
		refA[i] = refB[i] + s*refC[i] // triad
	}

	gpuA := make([]float32, n)
	gpuB := make([]float32, n)
	gpuC := make([]float32, n)
	b.driver.MemCopyD2H(b.context, gpuA, b.gA)
	b.driver.MemCopyD2H(b.context, gpuB, b.gB)
	b.driver.MemCopyD2H(b.context, gpuC, b.gC)

	check := func(name string, idx int, ref float64, got float32) {
		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-float64(got))/denom > 1e-3 {
			log.Fatalf("%s mismatch at %d, expected %f, but got %f.\n",
				name, idx, ref, got)
		}
	}

	for i := 0; i < n; i++ {
		check("a", i, refA[i], gpuA[i])
		check("b", i, refB[i], gpuB[i])
		check("c", i, refC[i], gpuC[i])
	}

	log.Printf("Passed!\n")
}
