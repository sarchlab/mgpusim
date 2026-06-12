// Package main runs the Parboil LBM benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/lbm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var gridDim = flag.Int("grid_dim", 32, "Grid dimension N (N^3 cells)")
var numTimesteps = flag.Int("num_timesteps", 10, "Number of LBM timesteps")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := lbm.NewBenchmark(r.Driver())
	benchmark.GridDim = *gridDim
	benchmark.NumTimesteps = *numTimesteps

	r.AddBenchmark(benchmark)
	r.Run()
}
