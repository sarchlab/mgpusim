package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/conv3d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var n = flag.Int("size", 32, "The volume edge length N (NxNxN volume).")
var filterSize = flag.Int("filter-size", 3, "Fixed convolution stencil extent (3).")

var blockSize = flag.Int("block-size", 8, "The edge length of a cubic work-group.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := conv3d.NewBenchmark(runtime)
	benchmark.N = *n
	benchmark.FilterSize = *filterSize
	benchmark.BlockSize = *blockSize
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
