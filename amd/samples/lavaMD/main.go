package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/lavaMD"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var boxesPerDim = flag.Int("boxes_per_dim", 3, "Number of boxes per dimension")
var parPerBox = flag.Int("par_per_box", 100, "Number of particles per box")
var cutoffRadius = flag.Float64("cutoff_radius", 10.0, "Cutoff radius")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := lavaMD.NewBenchmark(runner.Driver())
	benchmark.SetBoxesPerDim(*boxesPerDim)
	benchmark.SetParPerBox(*parPerBox)
	benchmark.SetCutoffRadius(float32(*cutoffRadius))

	runner.AddBenchmark(benchmark)

	runner.Run()
}
