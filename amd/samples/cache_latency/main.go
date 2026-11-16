package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/microbench/cachelatency"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var arrayBytes = flag.Int("array-bytes", 16*1024,
	"Size of the pointer-chasing array in bytes.")
var cachelineBytes = flag.Int("cacheline-bytes", 64,
	"Chase granularity: one chase node per cacheline (gfx942 line = 64 B).")
var measureLaps = flag.Int("measure-laps", 4,
	"Timed laps used to derive the access count when -num-accesses is 0.")
var numAccesses = flag.Int("num-accesses", 0,
	"Dependent loads to perform. 0 = derive from -measure-laps (match real HW).")
var seed = flag.Int("seed", 42, "RNG seed for the chain permutation.")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := cachelatency.NewBenchmark(runtime)
	benchmark.ArrayBytes = *arrayBytes
	benchmark.CachelineBytes = *cachelineBytes
	benchmark.MeasureLaps = *measureLaps
	benchmark.NumAccesses = *numAccesses
	benchmark.Seed = uint32(*seed)
	benchmark.Arch = config.ArchType

	config.Configure(benchmark)
	runner.AddBenchmark(benchmark)

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
