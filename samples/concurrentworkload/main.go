package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/bitonicsort"
	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/samples/runner"
)

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	firBenchmark := fir.NewBenchmark(runner.GPUDriver)
	firBenchmark.Length = 16384
	firBenchmark.SelectGPU(1)

	bsBenchmark := bitonicsort.NewBenchmark(runner.GPUDriver)
	bsBenchmark.Length = 1024
	bsBenchmark.SelectGPU(2)

	runner.AddBenchmark(firBenchmark)
	runner.AddBenchmark(bsBenchmark)

	runner.Run()
}
