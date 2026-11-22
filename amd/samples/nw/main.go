package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/nw"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var length = flag.Int("length", 64, "The number bases in the gene sequence")

var blockSize = flag.Int("block-size", 16, "Work-group width for the CDNA3 tiled kernel.")
var penalty = flag.Int("penalty", 10, "Gap penalty used by Needleman-Wunsch.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := nw.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType
	benchmark.SetLength(*length)
	benchmark.NativeBlockSize = *blockSize
	benchmark.SetPenalty(*penalty)

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
