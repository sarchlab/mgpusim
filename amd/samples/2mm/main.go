package main

import (
	"flag"

	mm2 "github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/2mm"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var niFlag = flag.Int("ni", 256, "The NI dimension.")
var njFlag = flag.Int("nj", 256, "The NJ dimension.")
var nkFlag = flag.Int("nk", 256, "The NK dimension.")
var nlFlag = flag.Int("nl", 256, "The NL dimension.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := mm2.NewBenchmark(runner.Driver())
	benchmark.NI = *niFlag
	benchmark.NJ = *njFlag
	benchmark.NK = *nkFlag
	benchmark.NL = *nlFlag

	runner.AddBenchmark(benchmark)

	runner.Run()
}
