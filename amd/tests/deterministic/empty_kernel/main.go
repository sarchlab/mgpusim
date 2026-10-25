package main

import (
	"flag"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
	"io"
	"log"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"

	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Benchmark defines a benchmark
type Benchmark struct {
	benchmarks.Progress
	queue   driver.CommandQueue
	gpu     int
	driver  *driver.Runtime
	context driver.Context

	hsaco *insts.KernelCodeObject

	useUnifiedMemory bool
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpu = gpus[0]
}

// Step submits the benchmark once and completes after the global drain.
func (b *Benchmark) submit() {
	b.context = b.driver.Init()
	b.driver.SelectGPU(b.context, b.gpu)
	b.queue = b.driver.CreateCommandQueue(b.context)
	b.loadProgram()
	b.initMem()
	b.exec()
}

// Verify verifies
func (Benchmark) Verify() {
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
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

	b.driver.EnqueueLaunchKernel(
		b.queue,
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
	b := &Benchmark{driver: runtime}
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
	if b.Phase == 0 {
		b.submit()
		b.Phase = 1
	} else {
		b.Ended = true
	}
	return nil
}

type checkpointState struct {
	Progress benchmarks.Progress
	Context  driver.Context
	Queue    driver.CommandQueue
}

func (b *Benchmark) config() [3]int { return [3]int{b.gpu, *numWfPerWGFlag, *numWGFlag} }
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.config(), checkpointState{b.Progress, b.context, b.queue}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.config(), &s, nil); err != nil {
		return err
	}
	b.Progress = s.Progress
	b.context = s.Context
	b.queue = s.Queue
	return nil
}

func main() {
	flag.Parse()

	t1 := run()
	t2 := run()

	log.Printf("t1: %d ps, t2: %d ps\n", t1, t2)

	if t1 != t2 {
		panic("non-deterministic behavior in empty kernel test")
	}
}
