// Command instruction_cost runs one kernel of the instruction_cost
// microbenchmark from sarchlab/gpu_benchmarks. The flags carry the
// gpu_benchmarks parameter names and defaults.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/loopkernels"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var instruction = flag.String("instruction", "v_fma_f32",
	fmt.Sprintf("What the loop body is made of: %v.", loopkernels.InstructionCostInstructions()))
var dependency = flag.String("dependency", "chained",
	"chained or independent; a scalar or branch body has one kernel for both.")
var opsPerIter = flag.Int("ops-per-iter", 4,
	"Instructions in the loop body: 0 (the loop control alone), 1, 2, 4, 8 or 16.")
var iterations = flag.Int("iterations", 65536, "Loop iterations per thread, at least 1.")
var numBlocks = flag.Int("num-blocks", 1, "The number of thread blocks to launch.")
var threadsPerBlock = flag.Int("threads-per-block", 64,
	"The work-group (block) size; must be <= 1024.")

func main() {
	flag.Parse()

	// Reject parameters that have no kernel before building the platform.
	err := loopkernels.CheckLaunch(*iterations, *numBlocks, *threadsPerBlock)
	if err == nil {
		_, err = loopkernels.InstructionCostKernel(*instruction, *dependency, *opsPerIter, *iterations)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := loopkernels.NewInstructionCost(runtime)
	benchmark.Instruction = *instruction
	benchmark.Dependency = *dependency
	benchmark.OpsPerIter = *opsPerIter
	benchmark.Iterations = *iterations
	benchmark.NumBlocks = *numBlocks
	benchmark.ThreadsPerBlock = *threadsPerBlock
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	if err := runner.Run(); err != nil {
		panic(err)
	}
	if err := finish(); err != nil {
		panic(err)
	}
}
