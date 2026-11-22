package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/srad"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var imageSize = flag.Int("size", 32,
	"The image dimension N (the image is N×N).")
var numIterations = flag.Int("iterations", 10,
	"The number of SRAD iterations to run.")

var lambda = flag.Float64("lambda", 0.5, "Diffusion coefficient.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := srad.NewBenchmark(runtime)
	benchmark.ImageSize = *imageSize
	benchmark.Lambda = float32(*lambda)
	benchmark.NumIterations = *numIterations
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
