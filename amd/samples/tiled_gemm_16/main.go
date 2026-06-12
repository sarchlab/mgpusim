package main

import (
	"flag"

	tiledgemm16 "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/tiled_gemm_16"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var mDim = flag.Int("m", 1024, "M dimension")
var nDim = flag.Int("n", 768, "N dimension")
var kDim = flag.Int("k", 768, "K dimension")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := tiledgemm16.NewBenchmark(runner.Driver())
	benchmark.SetM(*mDim)
	benchmark.SetN(*nDim)
	benchmark.SetK(*kDim)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
