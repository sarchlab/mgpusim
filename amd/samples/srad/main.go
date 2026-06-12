package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/srad"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var imageSize = flag.Int("image_size", 512, "Image size (rows = cols = image_size)")
var numIterations = flag.Int("num_iterations", 10, "Number of SRAD iterations")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := srad.NewBenchmark(runner.Driver())
	benchmark.SetImageSize(*imageSize)
	benchmark.SetNumIterations(*numIterations)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
