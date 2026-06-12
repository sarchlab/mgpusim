package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/huffman"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var dataSize = flag.Int("data_size", 1024, "Data size (number of bytes)")
var numSymbols = flag.Int("num_symbols", 256, "Number of symbols (max 256)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := huffman.NewBenchmark(runner.Driver())
	benchmark.SetDataSize(*dataSize)
	benchmark.SetNumSymbols(*numSymbols)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
