// Package blackscholes implements the Tango Black-Scholes benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/tango_blackscholes) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
//
// Each work-item prices one European option (call + put) using the
// Black-Scholes closed-form formula with a polynomial approximation of the
// cumulative normal distribution (Abramowitz & Stegun 26.2.17). The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300X configuration).
package blackscholes

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
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
	S         gpuruntime.Ptr // offset 0
	K         gpuruntime.Ptr // offset 8
	T         gpuruntime.Ptr // offset 16
	Sigma     gpuruntime.Ptr // offset 24
	R         float32        // offset 32
	Pad0      uint32         // offset 36 - alignment padding
	CallPrice gpuruntime.Ptr // offset 40
	PutPrice  gpuruntime.Ptr // offset 48
	N         int32          // offset 56
}

// Benchmark defines the Black-Scholes benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Black-Scholes benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the tango blackscholes benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
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

	b.HS = make([]float32, n)
	b.HK = make([]float32, n)
	b.HT = make([]float32, n)
	b.HSigma = make([]float32, n)

	var seed uint32 = 42
	for i := 0; i < n; i++ {
		b.HS[i] = randRange(&seed, 5.0, 200.0)
		b.HK[i] = randRange(&seed, 1.0, 300.0)
		b.HT[i] = randRange(&seed, 0.25, 10.0)
		b.HSigma[i] = randRange(&seed, 0.1, 1.0)
	}

	bytes := uint64(n * 4)
	if b.UseUnifiedMemory {
		b.GS = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GK = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GT = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GSigma = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GCall = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GPut = b.rt.AllocateUnifiedMemory(b.Context, bytes)
	} else {
		b.GS = b.rt.AllocateMemory(b.Context, bytes)
		b.GK = b.rt.AllocateMemory(b.Context, bytes)
		b.GT = b.rt.AllocateMemory(b.Context, bytes)
		b.GSigma = b.rt.AllocateMemory(b.Context, bytes)
		b.GCall = b.rt.AllocateMemory(b.Context, bytes)
		b.GPut = b.rt.AllocateMemory(b.Context, bytes)
	}

	b.Upload(b.rt, b.Queue, b.GS, b.HS)
	b.Upload(b.rt, b.Queue, b.GK, b.HK)
	b.Upload(b.rt, b.Queue, b.GT, b.HT)
	b.Upload(b.rt, b.Queue, b.GSigma, b.HSigma)
}

func (b *Benchmark) exec() {
	n := b.N
	gridSize := uint32((n + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize

	args := KernelArgs{
		S:         b.GS,
		K:         b.GK,
		T:         b.GT,
		Sigma:     b.GSigma,
		R:         riskFreeRate,
		CallPrice: b.GCall,
		PutPrice:  b.GPut,
		N:         int32(n),
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
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
func (b *Benchmark) verify() error {
	n := b.N
	gpuCall := make([]float32, n)
	gpuPut := make([]float32, n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuCall); err != nil {
		return err
	}
	if err := benchmarks.Read(b.rt, b.Readbacks[1], gpuPut); err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		s := b.HS[i]
		k := b.HK[i]
		t := b.HT[i]
		v := b.HSigma[i]

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
			return fmt.Errorf("At option %d, call expected %f, but got %f.\n",
				i, refCall, gpuCall[i])
		}
		if float32(math.Abs(float64(gpuPut[i]-refPut))) > tol {
			return fmt.Errorf("At option %d, put expected %f, but got %f.\n",
				i, refPut, gpuPut[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GCall, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GPut, 4*(b.N)))
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
