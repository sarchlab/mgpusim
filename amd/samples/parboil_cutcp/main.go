package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/cutcp"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var (
	numAtoms    = flag.Int("num-atoms", 64, "Number of atoms (point charges).")
	gridSide    = flag.Int("grid-side", 8, "Grid points per axis (grid is grid-side^3 points).")
	gridSpacing = flag.Float64("grid-spacing", 0.5, "Distance between adjacent grid points.")
	cutoff      = flag.Float64("cutoff", 12.0, "Interaction cutoff radius.")
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := cutcp.NewBenchmark(runner.Driver())
	benchmark.NumAtoms = *numAtoms
	benchmark.GridSide = *gridSide
	benchmark.GridSpacing = float32(*gridSpacing)
	benchmark.Cutoff = float32(*cutoff)
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
