package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/stencil2d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var numRow = flag.Int("row", 64, "The number of rows in the input matrix.")
var numCol = flag.Int("col", 64, "The number of columns in the input matrix.")
var numIter = flag.Int("iter", 1, "The number of iterations to run.")

var matrixSize = flag.Int("n", 0, "Square grid size including both boundaries.")
var blockSize = flag.Int("block-size", 16, "Work-group edge length for the CDNA3 square stencil.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := stencil2d.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType
	benchmark.NumIteration = *numIter
	benchmark.NumRows = *numRow + 2
	benchmark.NumCols = *numCol + 2
	if *matrixSize > 0 {
		benchmark.NumRows, benchmark.NumCols = *matrixSize, *matrixSize
	}
	benchmark.BlockSize = *blockSize

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
