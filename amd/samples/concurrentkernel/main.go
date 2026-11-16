package main

import (
	"flag"

	_ "net/http/pprof"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/bitonicsort"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	firBenchmark := fir.NewBenchmark(runtime)
	firBenchmark.Arch = config.ArchType
	firBenchmark.Length = 10240
	firBenchmark.SelectGPU([]int{1})

	bsBenchmark := bitonicsort.NewBenchmark(runtime)
	bsBenchmark.Arch = config.ArchType
	bsBenchmark.Length = 64
	bsBenchmark.SelectGPU([]int{1})

	config.ConfigureWithoutGPUs(firBenchmark)
	runner.AddBenchmark(firBenchmark)
	config.ConfigureWithoutGPUs(bsBenchmark)
	runner.AddBenchmark(bsBenchmark)

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
