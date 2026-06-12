// Package main runs the Parboil MRI Gridding benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/mri_gridding"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numSamples = flag.Int("num_samples", 65536, "Number of k-space samples")
var gridSize = flag.Int("grid_size", 256, "Grid dimension (grid_size x grid_size)")
var kernelWidth = flag.Int("kernel_width", 4, "Kaiser-Bessel kernel width")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := mri_gridding.NewBenchmark(r.Driver())
	benchmark.NumSamples = *numSamples
	benchmark.GridSize = *gridSize
	benchmark.KernelWidth = *kernelWidth

	r.AddBenchmark(benchmark)
	r.Run()
}
