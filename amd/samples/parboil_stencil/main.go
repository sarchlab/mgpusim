package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/stencil_parboil"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var gridDim = flag.Int("grid_dim", 32, "Grid dimension (Nx=Ny=Nz)")
var numTimesteps = flag.Int("num_timesteps", 10, "Number of stencil timesteps")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := stencil_parboil.NewBenchmark(runner.Driver())
	benchmark.GridDim = *gridDim
	benchmark.NumTimesteps = *numTimesteps

	runner.AddBenchmark(benchmark)

	runner.Run()
}
