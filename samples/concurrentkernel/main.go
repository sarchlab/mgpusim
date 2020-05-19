package main

import (
	"flag"

	_ "net/http/pprof"

	"gitlab.com/akita/mgpusim/benchmarks/amdappsdk/bitonicsort"
	"gitlab.com/akita/mgpusim/benchmarks/heteromark/fir"
	"gitlab.com/akita/mgpusim/samples/runner"
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	firBenchmark := fir.NewBenchmark(runner.GPUDriver)
	firBenchmark.Length = 10240
	firBenchmark.SelectGPU([]int{1})

	bsBenchmark := bitonicsort.NewBenchmark(runner.GPUDriver)
	bsBenchmark.Length = 64
	bsBenchmark.SelectGPU([]int{1})

	runner.AddBenchmarkWithoutSettingGPUsToUse(firBenchmark)
	runner.AddBenchmarkWithoutSettingGPUsToUse(bsBenchmark)

	runner.Run()
}
