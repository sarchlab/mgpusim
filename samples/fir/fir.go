package main

import (
	"flag"
	"log"

	"net/http"
	_ "net/http/pprof"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/samples/runner"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := fir.NewBenchmark(runner.GPUDriver)
	benchmark.Length = *numData

	runner.AddBenchmark(benchmark)

	runner.Run()
}
