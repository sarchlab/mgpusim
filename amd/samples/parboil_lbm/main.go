package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/lbm"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var gridDim = flag.Int("grid", 16,
	"The per-dimension grid size N (NxNxN D3Q19 lattice).")
var numTimesteps = flag.Int("timesteps", 4,
	"The number of collide-stream iterations to run.")
var tau = flag.Float64("tau", 0.7,
	"The relaxation time tau (omega = 1/tau).")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := lbm.NewBenchmark(runtime)
	benchmark.GridDim = *gridDim
	benchmark.NumTimesteps = *numTimesteps
	benchmark.Tau = float32(*tau)
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
