// Package codefetchlatency implements the code_fetch_latency microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/code_fetch_latency) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// One kernel is launched once. Each work-item runs a straight-line body of
// BodyOps dependent fused multiply-adds, Loops times, and stores the result.
// There is one kernel per body size, so the launch brings in only the code of
// the size under test. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3`.
package codefetchlatency

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"
	"slices"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/bitops"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Chain coefficients and the chain-length bound, identical to the HIP source.
const (
	fmaMul   float32 = 1.0000001
	fmaAdd   float32 = 0.0000001
	maxChain         = 1 << 20
)

// BodySizes lists every body size the benchmark defines: one kernel,
// fetch_kernel_<size>, per entry.
var BodySizes = []int{
	16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536,
}

// KernelArgs is the device ABI of fetch_kernel_<size>.
type KernelArgs struct {
	Out    gpuruntime.Ptr
	A, B   float32
	Reps   int32
	_      [4]byte
	Launch benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the code_fetch_latency benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new code_fetch_latency benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

// KernelName returns the kernel that holds a body of bodyOps operations.
func KernelName(bodyOps int) string {
	return fmt.Sprintf("fetch_kernel_%d", bodyOps)
}

func (b *Benchmark) loadProgram() {
	if !slices.Contains(BodySizes, b.bodyOps()) {
		log.Panicf("code_fetch_latency has no kernel with %d body operations; "+
			"supported sizes are %v", b.bodyOps(), BodySizes)
	}
	if b.loops() > maxChain/b.bodyOps() {
		log.Panicf("code_fetch_latency runs at most %d operations per thread; "+
			"%d body operations x %d loops exceeds it",
			maxChain, b.bodyOps(), b.loops())
	}

	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, KernelName(b.bodyOps()))
	if b.hsaco == nil {
		log.Panicf("Failed to load %s kernel binary", KernelName(b.bodyOps()))
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
		log.Panic("the code_fetch_latency benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) bodyOps() int {
	if b.BodyOps <= 0 {
		return 256
	}
	return b.BodyOps
}

func (b *Benchmark) loops() int {
	if b.Loops <= 0 {
		return 1
	}
	return b.Loops
}

func (b *Benchmark) numBlocks() int {
	if b.NumBlocks <= 0 {
		return 1
	}
	return b.NumBlocks
}

func (b *Benchmark) threadsPerBlock() int {
	if b.ThreadsPerBlock <= 0 {
		return 64
	}
	return b.ThreadsPerBlock
}

func (b *Benchmark) numThreads() int {
	return b.numBlocks() * b.threadsPerBlock()
}

func (b *Benchmark) initMem() {
	numElem := b.numThreads()
	outBytes := uint64(numElem * 4)

	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GOut = b.rt.AllocateMemory(b.Context, outBytes)
	}

	// As in the HIP source, start from NaN so that a work-item that never
	// stores fails verification.
	unwritten := make([]float32, numElem)
	for i := range unwritten {
		unwritten[i] = math.Float32frombits(0xFFFFFFFF)
	}
	b.Upload(b.rt, b.Queue, b.GOut, unwritten)
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.numThreads()), 1, 1}
	group := [3]uint16{uint16(b.threadsPerBlock()), 1, 1}
	args := KernelArgs{
		Out: b.GOut, A: fmaMul, B: fmaAdd, Reps: int32(b.loops()),
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// verify checks the GPU result against the same chain computed on the host.
//
// Each work-item stores the result of loops*bodyOps steps of x = x*a + b that
// start at 1 + threadIdx.x/1024. The seed is exact in float32 and every step
// is one correctly rounded FMA, so the comparison is bit for bit.
func (b *Benchmark) verify() error {
	numElem := b.numThreads()
	gpu := make([]float32, numElem)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpu); err != nil {
		return err
	}

	steps := b.bodyOps() * b.loops()
	tpb := b.threadsPerBlock()

	// The result depends only on threadIdx.x (== tid % block size).
	ref := make([]float32, tpb)
	for lane := range ref {
		x := bitops.FMAF32(float32(lane), 0.0009765625, 1)
		for range steps {
			x = bitops.FMAF32(x, fmaMul, fmaAdd)
		}
		ref[lane] = x
	}

	for tid := range numElem {
		want, got := ref[tid%tpb], gpu[tid]
		if math.Float32bits(got) != math.Float32bits(want) {
			return fmt.Errorf("At thread %d, expected %f, but got %f.\n",
				tid, want, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOut, 4*(b.numThreads())))
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
