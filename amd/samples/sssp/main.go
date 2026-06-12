package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/lonestar/sssp"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numNodes = flag.Int("num_nodes", 65536, "Number of nodes in the graph")
var maxIters = flag.Int("max_iters", 3, "Maximum relaxation iterations")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := sssp.NewBenchmark(r.Driver())
	benchmark.NumNodes = *numNodes
	benchmark.MaxIters = *maxIters

	r.AddBenchmark(benchmark)
	r.Run()
}
