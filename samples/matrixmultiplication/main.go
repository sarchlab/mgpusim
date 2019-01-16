package main

import (
	"flag"
	"log"
	"net/http"

	_ "net/http/pprof"

	"gitlab.com/akita/gcn3/benchmarks/amdappsdk/matrixmultiplication"
	"gitlab.com/akita/gcn3/samples/runner"
)

var lengthFlag = flag.Uint("length", 64, "The number of samples to filter.")

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := matrixmultiplication.NewBenchmark(runner.GPUDriver)
	benchmark.X = uint32(*lengthFlag)
	benchmark.Y = uint32(*lengthFlag)
	benchmark.Z = uint32(*lengthFlag)
	runner.Benchmark = benchmark

	runner.Run()

}
