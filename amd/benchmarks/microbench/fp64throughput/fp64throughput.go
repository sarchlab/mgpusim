// Package fp64throughput implements the GPU Benchmarks v1.1 register-resident
// FP64 throughput workload for MI300X (gfx942).
package fp64throughput

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

const (
	defaultThreadsPerBlock = 256
	fmaMul                 = 1.0000000001
	fmaAdd                 = 0.0000000001
)

// KernelArgs is the device ABI for the version 1.1 HIP kernel.
type KernelArgs struct {
	Out           gpuruntime.Ptr
	FmasPerThread int32
	_             [4]byte
	Launch        benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the fp64_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp64_throughput benchmark.
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
		cdna3HSACOBytes, "fp64_fma_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. fp64_throughput uses a single GPU.
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
		log.Panic("the fp64_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) normalizeParams() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 4
	}
	if b.FmasPerThread <= 0 {
		b.FmasPerThread = 16
	}
	if b.ThreadsPerBlock <= 0 {
		b.ThreadsPerBlock = defaultThreadsPerBlock
	}
	// Match the host program: round down to a multiple of 4, minimum 4.
	b.FmasPerThread = (b.FmasPerThread / 4) * 4
	if b.FmasPerThread == 0 {
		b.FmasPerThread = 4
	}

	b.NumThreads = b.NumBlocks * b.ThreadsPerBlock
}

func (b *Benchmark) initMem() {
	b.normalizeParams()

	outBytes := uint64(b.NumThreads * 8)
	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GOut = b.rt.AllocateMemory(b.Context, outBytes)
	}
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.NumThreads), 1, 1}
	group := [3]uint16{uint16(b.ThreadsPerBlock), 1, 1}
	args := KernelArgs{
		Out: b.GOut, FmasPerThread: int32(b.FmasPerThread),
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// cpuReference reproduces the four lane-specific FMA chains in the device kernel.
func (b *Benchmark) cpuReference(lane int) float64 {
	a0 := math.FMA(float64(lane), 0.001, 1)
	a1 := a0 + 0.1
	a2 := a0 + 0.2
	a3 := a0 + 0.3

	for i := 0; i < b.FmasPerThread; i += 4 {
		a0 = math.FMA(a0, fmaMul, fmaAdd)
		a1 = math.FMA(a1, fmaMul, fmaAdd)
		a2 = math.FMA(a2, fmaMul, fmaAdd)
		a3 = math.FMA(a3, fmaMul, fmaAdd)
	}

	return a0 + a1 + a2 + a3
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	gpuOut := make([]float64, b.NumThreads)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuOut); err != nil {
		return err
	}

	reference := make([]float64, b.ThreadsPerBlock)
	for lane := range reference {
		reference[lane] = b.cpuReference(lane)
	}
	for gid, got := range gpuOut {
		ref := reference[gid%b.ThreadsPerBlock]
		if math.IsNaN(got) || math.Abs(ref-got) > 1e-12*math.Abs(ref) {
			return fmt.Errorf("at work-item %d: expected %.12f, got %.12f", gid, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOut, 8*(b.NumThreads)))
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
