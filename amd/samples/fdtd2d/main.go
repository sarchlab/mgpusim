package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/fdtd2d"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var nxFlag = flag.Int("nx", 256, "The NX dimension.")
var nyFlag = flag.Int("ny", 256, "The NY dimension.")
var tmaxFlag = flag.Int("tmax", 10, "The number of timesteps.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := fdtd2d.NewBenchmark(runner.Driver())
	benchmark.NX = *nxFlag
	benchmark.NY = *nyFlag
	benchmark.TMAX = *tmaxFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
