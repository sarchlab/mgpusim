// Package binomialoptions implements the Tango Binomial Options benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/tango_binomial_options) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// It prices American put options with the Cox-Ross-Rubinstein (CRR) binomial
// tree model. One work-group (threadblock) prices one option; work-items
// collaborate via LDS (shared memory) on the backward induction through the
// tree. The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
//
// Each step writes to a different LDS buffer from the one it reads.
package binomialoptions

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

const maxNodes = 1025

// optionData mirrors the kernel's OptionData struct (5 contiguous float32s).
type optionData struct {
	S     float32 // stock price
	K     float32 // strike price
	T     float32 // time to expiration
	R     float32 // risk-free rate
	Sigma float32 // volatility
}

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
// Explicit arguments are followed by the gfx942 launch geometry.
type KernelArgs struct {
	Options  gpuruntime.Ptr // offset 0
	Prices   gpuruntime.Ptr // offset 8
	NumSteps int32          // offset 16
	_        uint32
	Launch   benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the Binomial Options benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Binomial Options benchmark.
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
		cdna3HSACOBytes, "binomial_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
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
		log.Panic("the tango binomial options benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// randRange reproduces the host LCG used by the original HIP benchmark so the
// device input is byte-for-byte the data the CPU reference re-derives.
func randRange(seed *uint32, lo, hi float32) float32 {
	*seed = (*seed)*1103515245 + 12345
	t := float32(*seed&0x7fffffff) / float32(0x7fffffff)
	// Round the product separately, as in the collected x86 host input generator.
	return lo + float32(t*(hi-lo))
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

	b.Options = make([]optionData, b.NumOptions)
	var seed uint32 = 42
	for i := 0; i < b.NumOptions; i++ {
		b.Options[i].S = randRange(&seed, 5.0, 200.0)
		b.Options[i].K = randRange(&seed, 1.0, 300.0)
		b.Options[i].T = randRange(&seed, 0.25, 10.0)
		b.Options[i].R = 0.02
		b.Options[i].Sigma = randRange(&seed, 0.1, 1.0)
	}

	optBytes := uint64(b.NumOptions * 20) // 5 float32 per option
	priceBytes := uint64(b.NumOptions * 4)

	if b.UseUnifiedMemory {
		b.GOptions = b.rt.AllocateUnifiedMemory(b.Context, optBytes)
		b.GPrices = b.rt.AllocateUnifiedMemory(b.Context, priceBytes)
	} else {
		b.GOptions = b.rt.AllocateMemory(b.Context, optBytes)
		b.GPrices = b.rt.AllocateMemory(b.Context, priceBytes)
	}

	b.Upload(b.rt, b.Queue, b.GOptions, b.Options)
}

func (b *Benchmark) exec() {
	// One work-group per option, fixed block size matching the kernel.
	threads := min(b.NumSteps+1, 1024)
	globalX := uint32(b.NumOptions * threads)
	group := [3]uint16{uint16(threads), 1, 1}

	args := KernelArgs{
		Options:  b.GOptions,
		Prices:   b.GPrices,
		NumSteps: int32(b.NumSteps),
		Launch:   benchmarks.NewGFX942LaunchDimensions([3]uint32{globalX, 1, 1}, group),
	}

	code := *b.hsaco
	code.GroupSegmentByteSize += uint32(2 * (b.NumSteps + 1) * 4)
	b.rt.EnqueueLaunchKernel(
		b.Queue,
		&code,
		[3]uint32{globalX, 1, 1},
		group,
		&args,
	)
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
func (b *Benchmark) verify() error {
	gpuPrices := make([]float32, b.NumOptions)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuPrices); err != nil {
		return err
	}

	for i := 0; i < b.NumOptions; i++ {
		ref := binomialCPU(b.Options[i], b.NumSteps)
		got := gpuPrices[i]

		diff := math.Abs(float64(got - ref))
		tol := 1e-2*math.Abs(float64(ref)) + 1e-4
		if math.IsInf(float64(got), 0) || math.IsInf(float64(ref), 0) || !(diff <= tol) {
			return fmt.Errorf("At option %d, expected %f, but got %f (diff=%f).\n",
				i, ref, got, diff)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GPrices, 4*(b.NumOptions)))
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
