package main

import (
	"flag"
	"log"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Benchmark defines a benchmark
type State struct {
	benchmarks.Progress
	Queue            gpuruntime.CommandQueue
	Context          gpuruntime.Context
	GPU              int
	UseUnifiedMemory bool
}
type Benchmark struct {
	*State
	benchmarks.Checkpoint[[3]int, State]
	rt    *gpuruntime.Runtime
	hsaco *insts.KernelCodeObject
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.GPU = gpus[0]
}

// Step submits the benchmark once and completes after the global drain.
func (b *Benchmark) submit() {
	b.Context = b.rt.Init()
	b.rt.SelectGPU(b.Context, b.GPU)
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.loadProgram()
	b.initMem()
	b.exec()
}

// Verify verifies
func (Benchmark) Verify() {
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromFS("kernels.hsaco", "")
}

func (b *Benchmark) initMem() {
}

func (b *Benchmark) exec() {
	kernArg := KernelArgs{
		0, 0, 0,
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{uint32(64 * *numWfPerWGFlag * *numWGFlag), 1, 1},
		[3]uint16{uint16(64 * *numWfPerWGFlag), 1, 1},
		&kernArg,
	)
}

var numWfPerWGFlag = flag.Int("num-wf-per-wg", 1, "The number of wavefronts in each workgroup")
var numWGFlag = flag.Int("num-wg", 1, "The number of workgroups in total")

func run() timing.VTimeInPicoSec {
	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	defer finish()
	r := runner.New(simulation, runtime)
	b := &Benchmark{State: &State{}, rt: runtime}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.config, func() error { b.loadProgram(); return nil })
	config.Configure(b)
	r.AddBenchmark(b)
	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := r.Run(); err != nil {
		panic(err)
	}
	return simulation.Engine().CurrentTime()
}
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if b.Phase == 0 {
		b.submit()
		b.Phase = 1
	} else {
		b.Ended = true
	}
	return nil
}

func (b *Benchmark) config() [3]int { return [3]int{b.GPU, *numWfPerWGFlag, *numWGFlag} }

func main() {
	flag.Parse()

	t1 := run()
	t2 := run()

	log.Printf("t1: %d ps, t2: %d ps\n", t1, t2)

	if t1 != t2 {
		panic("non-deterministic behavior in empty kernel test")
	}
}
