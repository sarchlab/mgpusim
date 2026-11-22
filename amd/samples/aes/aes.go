package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/aes"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var lenInput = flag.Int("length", 65536, "The number of input bytes, a multiple of 16.")

var blockSize = flag.Int("block-size", 256, "The number of work-items per group on CDNA3.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := aes.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType
	benchmark.Length = *lenInput
	benchmark.BlockSize = *blockSize

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
