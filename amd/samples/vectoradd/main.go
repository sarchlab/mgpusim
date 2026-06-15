package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/vectoradd"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var widthFlag = flag.Uint("width", 1024, "The width of the vectors.")
var heightFlag = flag.Uint("height", 1024, "The height of the vectors.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := vectoradd.NewBenchmark(runner.Driver())
	benchmark.Width = uint32(*widthFlag)
	benchmark.Height = uint32(*heightFlag)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
