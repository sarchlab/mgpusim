package main

import (
	"flag"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/tests/deterministic/runner"
)

type KernelArgs struct {
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context

	hsaco *insts.HsaCo

	useUnifiedMemory bool
}

func (b *Benchmark) SelectGPU(gpus []int) {
}

func (b *Benchmark) Run() {
	b.context = b.driver.Init()
	b.loadProgram()
	b.initMem()
	b.exec()
}

func (Benchmark) Verify() {
}

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.hsaco = kernels.LoadProgram("kernels.hsaco", "")
}

func (b *Benchmark) initMem() {
}

func (b *Benchmark) exec() {
	kernArg := KernelArgs{
		0, 0, 0,
	}

	b.driver.LaunchKernel(
		b.context,
		b.hsaco,
		[3]uint32{uint32(64 * *numWfPerWGFlag * *numWGFlag), 1, 1},
		[3]uint16{uint16(64 * *numWfPerWGFlag), 1, 1},
		&kernArg,
	)
}

var numWfPerWGFlag = flag.Int("num-wf-per-wg", 1, "The number of wavefronts in each workgroup")
var numWGFlag = flag.Int("num-wg", 1, "The number of workgroups in total")

func run() akita.VTimeInSec {
	runner := new(runner.Runner)
	runner.Init()

	benchmark := new(Benchmark)
	benchmark.driver = runner.GPUDriver
	runner.AddBenchmark(benchmark)

	runner.Run()

	return runner.Engine.CurrentTime()
}

func main() {
	flag.Parse()

	t1 := run()
	t2 := run()

	log.Printf("t1: %.10f, t2: %.10f\n", t1, t2)

	if t1 != t2 {
		panic("non-deterministic behavior in empty kernel test")
	}
}
