package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/fft"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var mb = flag.Int("MB", 8, "data size (in megabytes)")
var bytesFlag = flag.Int("bytes", 0, "data size in bytes (overrides -MB)")
var passes = flag.Int("passes", 1, "number of passes")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := fft.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType

	if *bytesFlag > 0 {
		benchmark.Bytes = int64(*bytesFlag)
		benchmark.BytesMode = true
	} else {
		benchmark.Bytes = int64(*mb)
	}

	benchmark.Passes = int32(*passes)

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
