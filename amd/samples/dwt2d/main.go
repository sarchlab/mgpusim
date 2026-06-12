package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/dwt2d"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var imageSize = flag.Int("image_size", 64, "Image dimension (NxN)")
var levels = flag.Int("levels", 3, "Number of decomposition levels")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := dwt2d.NewBenchmark(runner.Driver())
	benchmark.SetImageSize(*imageSize)
	benchmark.SetLevels(*levels)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
