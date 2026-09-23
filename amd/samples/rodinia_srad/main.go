package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/srad"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var imageSize = flag.Int("size", 32,
	"The image dimension N (the image is N×N).")
var numIterations = flag.Int("iterations", 10,
	"The number of SRAD iterations to run.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := srad.NewBenchmark(runner.Driver())
	benchmark.ImageSize = *imageSize
	benchmark.NumIterations = *numIterations
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
