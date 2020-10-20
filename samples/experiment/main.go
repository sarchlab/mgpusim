package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks"
	"gitlab.com/akita/mgpusim/benchmarks/heteromark/fir"
)

var benchmarkFlag = flag.String("benchmark", "",
	"Which benchmark to execute")

func main() {
	flag.Parse()

	runner := new(Runner).ParseFlag().Init()

	var benchmark benchmarks.Benchmark
	switch *benchmarkFlag {
	case "fir":
		firBenchmark := fir.NewBenchmark(runner.GPUDriver)
		firBenchmark.Length = 65536
		benchmark = firBenchmark
	}

	runner.AddBenchmark(benchmark)
	runner.Run()
}
