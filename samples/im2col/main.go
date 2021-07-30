package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/v2/benchmarks/dnn/im2col"
	"gitlab.com/akita/mgpusim/v2/samples/runner"
)

var n = flag.Int("N", 1, "batch size")
var c = flag.Int("C", 1, "input channels")
var h = flag.Int("H", 28, "input height")
var w = flag.Int("W", 28, "input width")
var kernelHeight = flag.Int("kernel-height", 3, "kernel height")
var kernelWidth = flag.Int("kernel-width", 3, "kernel width")
var padX = flag.Int("pad-x", 0, "padding height")
var padY = flag.Int("pad-y", 0, "padding width")
var strideX = flag.Int("stride-x", 1, "stride height")
var strideY = flag.Int("stride-y", 1, "stride width")
var dilateX = flag.Int("dilate-x", 1, "dilation on the x axis")
var dilateY = flag.Int("dilate-y", 1, "dilation on the y axis")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := im2col.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.C = *c
	benchmark.H = *h
	benchmark.W = *w
	benchmark.KernelWidth = *kernelWidth
	benchmark.KernelHeight = *kernelHeight
	benchmark.PadX = *padX
	benchmark.PadY = *padY
	benchmark.StrideX = *strideX
	benchmark.StrideY = *strideY
	benchmark.DilateX = *dilateX
	benchmark.DilateY = *dilateY

	runner.AddBenchmark(benchmark)

	runner.Run()
}
