package main

import "github.com/sarchlab/accelsimtracing/component"

func main() {
	// benchmark := component.BuildBenchmarkFromTrace("data/bfs-rodinia-2.0-ft")
	benchmark := component.NewBenchmarkForTest()

	platform := component.NewTickingPlatform()

	runner := component.NewRunner()
	runner.SetPlatform(platform)
	runner.AddBenchmark(benchmark)

	runner.Run()
}
