// Package binomialoptions implements the Tango Binomial Options benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/tango_binomial_options) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// It prices American put options with the Cox-Ross-Rubinstein (CRR) binomial
// tree model. One work-group (threadblock) prices one option; work-items
// collaborate via LDS (shared memory) on the backward induction through the
// tree. The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
//
// The native kernel uses a constant compile-time block size (BLOCK_SIZE=256)
// and a statically-sized LDS array (MAX_NODES=256) instead of the original's
// blockDim.x stride bound and dynamic shared memory. As a result the compiler
// emits no hidden ABI arguments (kernarg_segment_size = 20) and the launch
// must use block dim {256,1,1} with NumSteps+1 <= 256.
package binomialoptions

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize and maxNodes must match the native kernel (BLOCK_SIZE, MAX_NODES).
const (
	blockSize = 256
	maxNodes  = 256
)

// optionData mirrors the kernel's OptionData struct (5 contiguous float32s).
type optionData struct {
	S     float32 // stock price
	K     float32 // strike price
	T     float32 // time to expiration
	R     float32 // risk-free rate
	Sigma float32 // volatility
}

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// Layout verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 20): two 8-byte global_buffer pointers followed by
// one 4-byte by_value scalar, packed with no padding (mgpusim serializes args
// with binary.Write, which does not insert alignment padding). The kernel uses
// a constant block dimension, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Options  driver.Ptr // offset 0
	Prices   driver.Ptr // offset 8
	NumSteps int32      // offset 16
}

// Benchmark defines the Binomial Options benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch       arch.Type
	NumOptions int
	NumSteps   int

	options  []optionData
	gOptions driver.Ptr
	gPrices  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Binomial Options benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "binomial_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
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
		log.Panic("the tango binomial options benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// randRange reproduces the host LCG used by the original HIP benchmark so the
// device input is byte-for-byte the data the CPU reference re-derives.
func randRange(seed *uint32, lo, hi float32) float32 {
	*seed = (*seed)*1103515245 + 12345
	t := float32(*seed&0x7fffffff) / float32(0x7fffffff)
	return lo + t*(hi-lo)
}

func (b *Benchmark) initMem() {
	if b.NumOptions <= 0 {
		b.NumOptions = 8
	}
	if b.NumSteps <= 0 {
		b.NumSteps = 64
	}
	if b.NumSteps+1 > maxNodes {
		log.Panicf("NumSteps+1 (%d) exceeds kernel MAX_NODES (%d)",
			b.NumSteps+1, maxNodes)
	}

	b.options = make([]optionData, b.NumOptions)
	var seed uint32 = 42
	for i := 0; i < b.NumOptions; i++ {
		b.options[i].S = randRange(&seed, 5.0, 200.0)
		b.options[i].K = randRange(&seed, 1.0, 300.0)
		b.options[i].T = randRange(&seed, 0.25, 10.0)
		b.options[i].R = 0.02
		b.options[i].Sigma = randRange(&seed, 0.1, 1.0)
	}

	optBytes := uint64(b.NumOptions * 20) // 5 float32 per option
	priceBytes := uint64(b.NumOptions * 4)

	if b.useUnifiedMemory {
		b.gOptions = b.driver.AllocateUnifiedMemory(b.context, optBytes)
		b.gPrices = b.driver.AllocateUnifiedMemory(b.context, priceBytes)
	} else {
		b.gOptions = b.driver.AllocateMemory(b.context, optBytes)
		b.gPrices = b.driver.AllocateMemory(b.context, priceBytes)
	}

	b.driver.MemCopyH2D(b.context, b.gOptions, b.options)
}

func (b *Benchmark) exec() {
	// One work-group per option, fixed block size matching the kernel.
	globalX := uint32(b.NumOptions * blockSize)

	args := KernelArgs{
		Options:  b.gOptions,
		Prices:   b.gPrices,
		NumSteps: int32(b.NumSteps),
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

// binomialCPU is the float32 reference, identical in arithmetic order to the
// device kernel, so results match to within rounding.
func binomialCPU(opt optionData, numSteps int) float32 {
	dt := opt.T / float32(numSteps)
	u := f32exp(opt.Sigma * f32sqrt(dt))
	d := float32(1.0) / u
	r := f32exp(opt.R * dt)
	rinv := float32(1.0) / r
	p := (r - d) / (u - d)
	q := float32(1.0) - p

	vals := make([]float32, numSteps+1)
	for j := 0; j <= numSteps; j++ {
		st := opt.S * f32pow(u, float32(2*j-numSteps))
		vals[j] = f32max(opt.K-st, 0.0)
	}

	for step := numSteps; step > 0; step-- {
		for j := 0; j < step; j++ {
			cont := rinv * (p*vals[j+1] + q*vals[j])
			st := opt.S * f32pow(u, float32(2*j-(step-1)))
			exercise := f32max(opt.K-st, 0.0)
			vals[j] = f32max(cont, exercise)
		}
	}

	return vals[0]
}

func f32exp(x float32) float32  { return float32(math.Exp(float64(x))) }
func f32sqrt(x float32) float32 { return float32(math.Sqrt(float64(x))) }
func f32pow(b, e float32) float32 {
	return float32(math.Pow(float64(b), float64(e)))
}
func f32max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	gpuPrices := make([]float32, b.NumOptions)
	b.driver.MemCopyD2H(b.context, gpuPrices, b.gPrices)

	for i := 0; i < b.NumOptions; i++ {
		ref := binomialCPU(b.options[i], b.NumSteps)
		got := gpuPrices[i]

		diff := math.Abs(float64(got - ref))
		tol := 1e-2*math.Abs(float64(ref)) + 1e-3
		if diff > tol {
			log.Fatalf("At option %d, expected %f, but got %f (diff=%f).\n",
				i, ref, got, diff)
		}
	}

	log.Printf("Passed!\n")
}
