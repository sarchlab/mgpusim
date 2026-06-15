package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/fft"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var mb = flag.Int("MB", 8, "data size (in megabytes)")
var bytesFlag = flag.Int("bytes", 0, "data size in bytes (overrides -MB)")
var passes = flag.Int("passes", 1, "number of passes")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fft.NewBenchmark(runner.Driver())
	benchmark.Arch = runner.ArchType

	if *bytesFlag > 0 {
		benchmark.Bytes = int64(*bytesFlag)
		benchmark.BytesMode = true
	} else {
		benchmark.Bytes = int64(*mb)
	}

	benchmark.Passes = int32(*passes)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
