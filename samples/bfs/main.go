package main

import (
	"flag"
	"math"

	"gitlab.com/akita/mgpusim/benchmarks/shoc/bfs"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var path = flag.String("load-graph", "", "Path to file from which graph to be loaded.")
var numNode = flag.Int("node", 64, "The width of the matrix.")
var degree = flag.Int("degree", 3, "The height of the matrix.")
var maxDepth = flag.Int("depth", 0, "The max depth to search, 0 means unlimited")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := bfs.NewBenchmark(runner.GPUDriver)
	benchmark.Path = *path
	benchmark.NumNode = *numNode
	benchmark.Degree = *degree
	if *maxDepth == 0 {
		*maxDepth = math.MaxInt32
	}
	benchmark.MaxDepth = *maxDepth

	runner.AddBenchmark(benchmark)

	runner.Run()
}
