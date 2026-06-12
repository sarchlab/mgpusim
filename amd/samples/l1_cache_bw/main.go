package main

import (
	"flag"

	l1cachebw "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/l1_cache_bw"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int(
	"num_elements", 1048576,
	"Problem-size work axis: output elements/threads; L1 simulator input "+
		"uses 256-float read groups, not a fixed total working_set_size; "+
		"total_read_bytes = num_elements * num_repeats * 4",
)
var numRepeats = flag.Int(
	"num_repeats", 256,
	"Simulator name for HIP/CUDA num_iterations; repeated float32 reads per output element",
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := l1cachebw.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)
	benchmark.SetNumRepeats(*numRepeats)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
