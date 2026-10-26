package main

import (
	"flag"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/bfs"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var path = flag.String("load-graph", "", "Path to file from which graph to be loaded. "+
	"Currently only supports text files.\nThe graph is considered directed and edges are "+
	"needed to described in single line \nwith format: <node from> <node to>. You can add "+
	"comment preceded by #")
var numNode = flag.Int("node", 64, "The width of the matrix.")
var degree = flag.Int("degree", 3, "The height of the matrix.")
var maxDepth = flag.Int("depth", 0, "The max depth to search, 0 means unlimited")

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := bfs.NewBenchmark(runtime)
	benchmark.Arch = config.ArchType
	benchmark.Path = *path
	benchmark.NumNode = *numNode
	benchmark.Degree = *degree
	if *maxDepth == 0 {
		*maxDepth = math.MaxInt32
	}
	benchmark.MaxDepth = *maxDepth

	if (isFlagPassed("degree") || isFlagPassed("node")) && isFlagPassed("load-graph") {
		log.Panic("cannot specify number or degree of nodes for manually provided graph")
	}

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

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
