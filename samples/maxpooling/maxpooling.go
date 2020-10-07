package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/maxpooling"
	"gitlab.com/akita/mgpusim/samples/runner"
)

var n = flag.Int("n", 1, "Batch size.")
var c = flag.Int("c", 1, "Channel size.")
var h = flag.Int("h", 32, "Height.")
var w = flag.Int("w", 32, "Weight.")
var kernelH = flag.Int("kernel-h", 3, "Kernel height.")
var kernelW = flag.Int("kernel-w", 3, "Kernel width.")
var padH = flag.Int("pad-h", 0, "Padding height.")
var padW = flag.Int("pad-w", 0, "Padding width.")
var strideH = flag.Int("stride-h", 1, "Stride on the y-axis.")
var strideW = flag.Int("stride-w", 1, "Stride on the x-axis.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	parameter := maxpooling.Parameters{
		N:       *n,
		C:       *c,
		H:       *h,
		W:       *w,
		KernelH: *kernelH,
		KernelW: *kernelW,
		PadH:    *padH,
		PadW:    *padW,
		StrideH: *strideH,
		StrideW: *strideW,
	}
	benchmark := maxpooling.NewBenchmark(
		runner.GPUDriver, parameter)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
