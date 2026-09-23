package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/altis/raytracing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var width = flag.Int("width", 64, "The image width in pixels.")
var height = flag.Int("height", 64, "The image height in pixels.")
var spheres = flag.Int("spheres", 16, "The number of spheres in the scene.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := raytracing.NewBenchmark(runner.Driver())
	benchmark.Width = *width
	benchmark.Height = *height
	benchmark.Spheres = *spheres
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
