package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/spmv"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

// Dim is dimension
var Dim = flag.Int("dim", 128, "The number of rows in the input matrix.")

// Sparsity is sparsity
var Sparsity = flag.Float64("sparsity", 0.01,
	"The ratio between non-zero elements to all the elelements in the matrix")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := spmv.NewBenchmark(runtime)
	benchmark.Dim = int32(*Dim)
	benchmark.Sparsity = *Sparsity
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
