package main

import (
	"flag"

	"gitlab.com/akita/mgpusim/v3/benchmarks/dnn/conv2d"
	"gitlab.com/akita/mgpusim/v3/samples/runner"
)

var n = flag.Int("N", 1, "batch size")
var c = flag.Int("C", 1, "input channels")
var h = flag.Int("H", 28, "input height")
var w = flag.Int("W", 28, "input width")
var outputC = flag.Int("output-channel", 3, "output channels")
var kernelHeight = flag.Int("kernel-height", 3, "kernel height")
var kernelWidth = flag.Int("kernel-width", 3, "kernel width")
var padX = flag.Int("pad-x", 0, "padding height")
var padY = flag.Int("pad-y", 0, "padding width")
var strideX = flag.Int("stride-x", 1, "stride height")
var strideY = flag.Int("stride-y", 1, "stride width")
var enableBackward = flag.Bool("enable-backward", false, "enable backward")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := conv2d.NewBenchmark(runner.Driver())
	benchmark.N = *n
	benchmark.C = *c
	benchmark.H = *h
	benchmark.W = *w
	benchmark.KernelChannel = *outputC
	benchmark.KernelWidth = *kernelWidth
	benchmark.KernelHeight = *kernelHeight
	benchmark.PadX = *padX
	benchmark.PadY = *padY
	benchmark.StrideX = *strideX
	benchmark.StrideY = *strideY
	benchmark.EnableBackward = *enableBackward

	runner.AddBenchmark(benchmark)

	runner.Run()
}
