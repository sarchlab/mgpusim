package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/altis/cfd"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var n = flag.Int("size", 256, "The number of mesh cells N.")

var blockSize = flag.Int("block-size", 256, "Cells processed per work-group.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := cfd.NewBenchmark(runtime)
	benchmark.N = *n
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
