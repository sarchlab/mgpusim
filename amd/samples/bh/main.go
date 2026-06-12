package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/lonestar/bh"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numBodies = flag.Int("num_bodies", 16384, "Number of bodies")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := bh.NewBenchmark(r.Driver())
	benchmark.NumBodies = *numBodies

	r.AddBenchmark(benchmark)
	r.Run()
}
