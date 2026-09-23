package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/lbm"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var gridDim = flag.Int("grid", 16,
	"The per-dimension grid size N (NxNxN D3Q19 lattice).")
var numTimesteps = flag.Int("timesteps", 4,
	"The number of collide-stream iterations to run.")
var tau = flag.Float64("tau", 0.7,
	"The relaxation time tau (omega = 1/tau).")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := lbm.NewBenchmark(runner.Driver())
	benchmark.GridDim = *gridDim
	benchmark.NumTimesteps = *numTimesteps
	benchmark.Tau = float32(*tau)
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
