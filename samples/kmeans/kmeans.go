package main

import (
	"flag"

	"gitlab.com/akita/gcn3/benchmarks/heteromark/kmeans"
	"gitlab.com/akita/gcn3/samples/runner"
)

var points = flag.Int("points", 1024, "The number of points.")
var clusters = flag.Int("clusters", 5, "The number of clusters.")
var features = flag.Int("features", 32,
	"The number of features for each point.")
var maxIter = flag.Int("max-iter", 5,
	"The maximum number of iterations to run")

func main() {
	flag.Parse()

	runner := runner.Runner{}
	runner.Init()

	benchmark := kmeans.NewBenchmark(runner.GPUDriver)
	benchmark.NumPoints = *points
	benchmark.NumClusters = *clusters
	benchmark.NumFeatures = *features
	benchmark.MaxIter = *maxIter

	runner.AddBenchmark(benchmark)

	runner.Run()
}
