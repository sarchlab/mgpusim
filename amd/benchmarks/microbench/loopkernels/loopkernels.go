// Package loopkernels runs the hand-written gfx942 loop kernels of the
// instruction_cost and loop_wait microbenchmarks of sarchlab/gpu_benchmarks
// (tier1/instruction_cost, tier1/loop_wait, and their shared
// common_hip/gfx942_loop_{kernels,host}.h) on the MGPUSim MI300X (CDNA3 /
// gfx942) model.
//
// Every kernel has the signature (uint64_t *out, int iterations), runs a loop
// of known instructions `iterations` times, and stores one 64-bit result per
// thread. The benchmark launches the selected kernel once and, with
// verification, compares every thread's result with a CPU reference.
//
// The three embedded code objects are the exact bytes the MI300X hardware
// timing collection ran: ROCm 10.2.0 builds of gpu_benchmarks commit cd5f922
// (kernel sources identical to main 8355c041). Their sources are not copied
// here; rebuild them from gpu_benchmarks. They hold gfx942 code only, so the
// benchmark must be run with `-arch cdna3`.
package loopkernels

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

// The benchmarks this package runs: the value of State.Benchmark.
const (
	InstructionCost = "instruction_cost"
	LoopWait        = "loop_wait"
)

var (
	//go:embed instruction_cost_kernels.hsaco
	instructionCostHSACO []byte
	//go:embed instruction_cost_other_kernels.hsaco
	instructionCostOtherHSACO []byte
	//go:embed loop_wait_kernels.hsaco
	loopWaitHSACO []byte
)

var codeObjects = map[string][]byte{
	InstructionCostObject:      instructionCostHSACO,
	InstructionCostOtherObject: instructionCostOtherHSACO,
	LoopWaitObject:             loopWaitHSACO,
}

// KernelArgs is the 272-byte kernarg segment hipcc lays out for
// (uint64_t *out, int iterations). The kernels read out, iterations and
// hidden_group_size_x.
type KernelArgs struct {
	Out        gpuruntime.Ptr
	Iterations int32
	_          [4]byte
	Launch     benchmarks.GFX942LaunchDimensions
	_          [16]byte
	// GlobalOffset is hidden_global_offset_x/y/z.
	GlobalOffset [3]uint64
	// GridDims is hidden_grid_dims.
	GridDims uint16
	_        [190]byte
}

// Benchmark launches one loop kernel.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	kernel Kernel
	hsaco  *insts.KernelCodeObject
}

// NewInstructionCost returns an instruction_cost benchmark with the
// gpu_benchmarks defaults.
func NewInstructionCost(rt *gpuruntime.Runtime) *Benchmark {
	b := newBenchmark(rt, InstructionCost)
	b.Dependency = "chained"
	return b
}

// NewLoopWait returns a loop_wait benchmark with the gpu_benchmarks defaults.
func NewLoopWait(rt *gpuruntime.Runtime) *Benchmark {
	b := newBenchmark(rt, LoopWait)
	b.Layout = "head_offset_8"
	return b
}

func newBenchmark(rt *gpuruntime.Runtime, benchmark string) *Benchmark {
	b := new(Benchmark)
	b.State = &State{
		Benchmark:       benchmark,
		Instruction:     "v_fma_f32",
		OpsPerIter:      4,
		Iterations:      65536,
		NumBlocks:       1,
		ThreadsPerBlock: 64,
	}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

// SelectKernel returns the kernel of the current parameters, or why they
// cannot be run.
func (b *Benchmark) SelectKernel() (Kernel, error) {
	if err := CheckLaunch(b.Iterations, b.NumBlocks, b.ThreadsPerBlock); err != nil {
		return Kernel{}, err
	}
	switch b.Benchmark {
	case InstructionCost:
		return InstructionCostKernel(b.Instruction, b.Dependency, b.OpsPerIter, b.Iterations)
	case LoopWait:
		return LoopWaitKernel(b.Layout, b.Instruction, b.OpsPerIter, b.Iterations)
	}
	return Kernel{}, fmt.Errorf("unknown loop benchmark %q", b.Benchmark)
}

// loadProgram loads the kernel of the current parameters. It keeps the kernel
// it already holds when the parameters cannot be run.
func (b *Benchmark) loadProgram() error {
	kernel, err := b.SelectKernel()
	if err != nil {
		return err
	}
	hsaco := insts.LoadKernelCodeObjectFromBytes(codeObjects[kernel.Object], kernel.Name)
	if hsaco == nil {
		return fmt.Errorf("failed to load %s from %s", kernel.Name, kernel.Object)
	}
	b.kernel = kernel
	b.hsaco = hsaco
	return nil
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

func (b *Benchmark) numThreads() int {
	return b.NumBlocks * b.ThreadsPerBlock
}

func (b *Benchmark) submit() error {
	if b.Arch != arch.CDNA3 {
		return fmt.Errorf("the %s benchmark ships only gfx942 kernels; "+
			"run with -arch cdna3 -gpu mi300x", b.Benchmark)
	}

	if err := b.loadProgram(); err != nil {
		return err
	}
	log.Printf("%s: kernel %s, %d iterations, %d blocks x %d threads\n",
		b.Benchmark, b.kernel.Name, b.Iterations, b.NumBlocks, b.ThreadsPerBlock)

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
	return nil
}

func (b *Benchmark) initMem() {
	numElem := b.numThreads()
	outBytes := uint64(numElem) * 8

	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GOut = b.rt.AllocateMemory(b.Context, outBytes)
	}

	// As in the HIP host, a sentinel no kernel stores, so a thread that
	// stored nothing is caught.
	unwritten := make([]uint64, numElem)
	for i := range unwritten {
		unwritten[i] = math.MaxUint64
	}
	b.Upload(b.rt, b.Queue, b.GOut, unwritten)
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.numThreads()), 1, 1}
	group := [3]uint16{uint16(b.ThreadsPerBlock), 1, 1}
	args := KernelArgs{
		Out:        b.GOut,
		Iterations: int32(b.Iterations),
		Launch:     benchmarks.NewGFX942LaunchDimensions(grid, group),
		GridDims:   1,
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// verify requires every thread to have stored the CPU reference.
func (b *Benchmark) verify() error {
	gpu := make([]uint64, b.numThreads())
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpu); err != nil {
		return err
	}

	want := b.kernel.Expected
	for tid, got := range gpu {
		if got != want {
			return fmt.Errorf("%s: thread %d stored 0x%016x, expected 0x%016x",
				b.kernel.Name, tid, got, want)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks,
		benchmarks.Download(b.rt, b.Queue, b.GOut, 8*b.numThreads()))
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
		if err := b.submit(); err != nil {
			return err
		}
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
