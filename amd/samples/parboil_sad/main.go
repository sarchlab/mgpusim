// Package main runs the Parboil SAD (Sum of Absolute Differences) benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/sad"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var frameWidth = flag.Int("frame_width", 256, "Frame width (and height) in pixels")
var blockSizeSAD = flag.Int("block_size_sad", 16, "Macroblock size for SAD computation")
var searchRange = flag.Int("search_range", 16, "Search range for motion estimation")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := sad.NewBenchmark(r.Driver())
	benchmark.FrameWidth = *frameWidth
	benchmark.BlockSizeSAD = *blockSizeSAD
	benchmark.SearchRange = *searchRange

	r.AddBenchmark(benchmark)
	r.Run()
}
