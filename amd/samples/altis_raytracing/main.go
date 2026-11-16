package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/altis/raytracing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var width = flag.Int("width", 64, "The image width in pixels.")
var height = flag.Int("height", 64, "The image height in pixels.")
var spheres = flag.Int("spheres", 16, "The number of spheres in the scene.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := raytracing.NewBenchmark(runtime)
	benchmark.Width = *width
	benchmark.Height = *height
	benchmark.Spheres = *spheres
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
