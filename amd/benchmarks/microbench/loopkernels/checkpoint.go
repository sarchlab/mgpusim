package loopkernels

import (
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// State is the saved continuation of a loop-kernel benchmark. The parameter
// fields carry the gpu_benchmarks parameter names.
type State struct {
	benchmarks.Progress
	Readbacks []gpuruntime.HostBuffer
	Context   gpuruntime.Context
	Queue     gpuruntime.CommandQueue
	Gpus      []int
	Arch      arch.Type

	// Benchmark is InstructionCost or LoopWait.
	Benchmark string
	// Instruction is what the loop body is made of.
	Instruction string
	// Dependency is chained or independent (instruction_cost only).
	Dependency string
	// Layout is how the loop is laid out (loop_wait only).
	Layout string
	// OpsPerIter is the number of instructions in the loop body.
	OpsPerIter int
	// Iterations is how many times each thread runs the loop, at least 1.
	Iterations int
	// NumBlocks is the number of thread blocks to launch.
	NumBlocks int
	// ThreadsPerBlock is the work-group size, at most 1024.
	ThreadsPerBlock int

	GOut             gpuruntime.Ptr
	UseUnifiedMemory bool
}

// bind reloads the kernel of the saved parameters. The expected result is
// derived from them as well, so it is not saved.
func (b *Benchmark) bind() error {
	return b.loadProgram()
}
