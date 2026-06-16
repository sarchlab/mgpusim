package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/cachelatency"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

var arrayBytes = flag.Int("array-bytes", 16*1024,
	"Size of the pointer-chasing array in bytes.")
var numAccesses = flag.Int("num-accesses", 1024,
	"Number of dependent loads the single thread performs.")
var seed = flag.Int("seed", 42, "RNG seed for the chain permutation.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := cachelatency.NewBenchmark(runner.Driver())
	benchmark.ArrayBytes = *arrayBytes
	benchmark.NumAccesses = *numAccesses
	benchmark.Seed = uint32(*seed)
	benchmark.Arch = runner.ArchType

	runner.AddBenchmark(benchmark)

	runner.Run()
}
