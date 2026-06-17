// Package blackscholes implements the Tango Black-Scholes benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/tango_blackscholes) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// Each work-item prices one European option (call + put) using the
// Black-Scholes closed-form formula with a polynomial approximation of the
// cumulative normal distribution (Abramowitz & Stegun 26.2.17). The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300A configuration).
package blackscholes

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the work-group size baked into the kernel (BLOCK_SIZE in
// native/tango_blackscholes.cpp). Using a compile-time constant lets the
// compiler emit no hidden ABI arguments.
const blockSize = 256

// riskFreeRate is the constant risk-free interest rate (matches the HIP host).
const riskFreeRate float32 = 0.02

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 60). mgpusim serializes args with binary.Write,
// which packs fields in declaration order with NO automatic alignment
// padding, so an explicit pad field is inserted at offset 36 to align the
// callPrice pointer to its metadata offset of 40. The kernel reads only
// blockIdx/threadIdx (constant block size), so no hidden ABI arguments are
// emitted.
type KernelArgs struct {
	S         driver.Ptr // offset 0
	K         driver.Ptr // offset 8
	T         driver.Ptr // offset 16
	Sigma     driver.Ptr // offset 24
	R         float32    // offset 32
	Pad0      uint32     // offset 36 - alignment padding
	CallPrice driver.Ptr // offset 40
	PutPrice  driver.Ptr // offset 48
	N         int32      // offset 56
}

// Benchmark defines the Black-Scholes benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	N    int

	hS     []float32
	hK     []float32
	hT     []float32
	hSigma []float32

	gS     driver.Ptr
	gK     driver.Ptr
	gT     driver.Ptr
	gSigma driver.Ptr
	gCall  driver.Ptr
	gPut   driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Black-Scholes benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "blackscholes_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Black-Scholes uses a single GPU.
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
		log.Panic("the tango blackscholes benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// randRange reproduces the HIP host's deterministic LCG, returning a float32
// in [lo, hi]. seed is advanced in place using uint32 wraparound arithmetic.
func randRange(seed *uint32, lo, hi float32) float32 {
	*seed = *seed*1103515245 + 12345
	t := float32(*seed&0x7fffffff) / float32(0x7fffffff)
	return lo + t*(hi-lo)
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 4096
	}

	n := b.N

	b.hS = make([]float32, n)
	b.hK = make([]float32, n)
	b.hT = make([]float32, n)
	b.hSigma = make([]float32, n)

	var seed uint32 = 42
	for i := 0; i < n; i++ {
		b.hS[i] = randRange(&seed, 5.0, 200.0)
		b.hK[i] = randRange(&seed, 1.0, 300.0)
		b.hT[i] = randRange(&seed, 0.25, 10.0)
		b.hSigma[i] = randRange(&seed, 0.1, 1.0)
	}

	bytes := uint64(n * 4)
	if b.useUnifiedMemory {
		b.gS = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gK = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gT = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gSigma = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gCall = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gPut = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.gS = b.driver.AllocateMemory(b.context, bytes)
		b.gK = b.driver.AllocateMemory(b.context, bytes)
		b.gT = b.driver.AllocateMemory(b.context, bytes)
		b.gSigma = b.driver.AllocateMemory(b.context, bytes)
		b.gCall = b.driver.AllocateMemory(b.context, bytes)
		b.gPut = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.gS, b.hS)
	b.driver.MemCopyH2D(b.context, b.gK, b.hK)
	b.driver.MemCopyH2D(b.context, b.gT, b.hT)
	b.driver.MemCopyH2D(b.context, b.gSigma, b.hSigma)
}

func (b *Benchmark) exec() {
	n := b.N
	gridSize := uint32((n + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize

	args := KernelArgs{
		S:         b.gS,
		K:         b.gK,
		T:         b.gT,
		Sigma:     b.gSigma,
		R:         riskFreeRate,
		CallPrice: b.gCall,
		PutPrice:  b.gPut,
		N:         int32(n),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// cnd reproduces the device cumulative-normal-distribution approximation in
// float32 so the CPU reference matches the GPU result bit-for-bit closely.
func cnd(d float32) float32 {
	const (
		a1       float32 = 0.31938153
		a2       float32 = -0.356563782
		a3       float32 = 1.781477937
		a4       float32 = -1.821255978
		a5       float32 = 1.330274429
		rsqrt2pi float32 = 0.39894228040143267793994605993438
	)

	k := float32(1.0) / (1.0 + 0.2316419*float32(math.Abs(float64(d))))
	cndVal := rsqrt2pi * float32(math.Exp(float64(-0.5*d*d))) *
		(k * (a1 + k*(a2+k*(a3+k*(a4+k*a5)))))

	if d > 0.0 {
		cndVal = 1.0 - cndVal
	}
	return cndVal
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.N
	gpuCall := make([]float32, n)
	gpuPut := make([]float32, n)
	b.driver.MemCopyD2H(b.context, gpuCall, b.gCall)
	b.driver.MemCopyD2H(b.context, gpuPut, b.gPut)

	for i := 0; i < n; i++ {
		s := b.hS[i]
		k := b.hK[i]
		t := b.hT[i]
		v := b.hSigma[i]

		sqrtT := float32(math.Sqrt(float64(t)))
		d1 := (float32(math.Log(float64(s/k))) +
			(riskFreeRate+0.5*v*v)*t) / (v * sqrtT)
		d2 := d1 - v*sqrtT
		expRT := float32(math.Exp(float64(-riskFreeRate * t)))
		cd1 := cnd(d1)
		cd2 := cnd(d2)

		refCall := s*cd1 - k*expRT*cd2
		refPut := k*expRT*(1.0-cd2) - s*(1.0-cd1)

		tol := 1e-3*(float32(math.Abs(float64(refCall)))+
			float32(math.Abs(float64(refPut)))) + 1e-3

		if float32(math.Abs(float64(gpuCall[i]-refCall))) > tol {
			log.Fatalf("At option %d, call expected %f, but got %f.\n",
				i, refCall, gpuCall[i])
		}
		if float32(math.Abs(float64(gpuPut[i]-refPut))) > tol {
			log.Fatalf("At option %d, put expected %f, but got %f.\n",
				i, refPut, gpuPut[i])
		}
	}

	log.Printf("Passed!\n")
}
