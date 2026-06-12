package main

import (
	"flag"

	s3d "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/s3d"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numElements = flag.Int("num_elements", 4096, "Number of grid cells")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := s3d.NewBenchmark(runner.Driver())
	benchmark.SetNumElements(*numElements)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
