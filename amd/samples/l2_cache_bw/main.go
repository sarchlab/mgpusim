package main

import (
	"flag"

	l2cachebw "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/l2_cache_bw"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int(
	"num_elements", 1048576,
	"Problem-size work axis: output elements/threads; "+
		"total_read_bytes = num_elements * num_repeats * 4 "+
		"(num_repeats corresponds to HIP/CUDA num_iterations)",
)
var numRepeats = flag.Int(
	"num_repeats", 128,
	"Simulator name for HIP/CUDA num_iterations; repeated float32 reads per output element",
)
var workingSetSize = flag.Int(
	"working_set_size", 262144,
	"Non-scaling L2-resident input footprint in bytes",
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := l2cachebw.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)
	benchmark.SetNumRepeats(*numRepeats)
	benchmark.SetWorkingSetSizeBytes(*workingSetSize)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
