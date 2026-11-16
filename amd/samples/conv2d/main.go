package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layer_benchmarks/conv2d"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
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

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := conv2d.NewBenchmark(runtime)
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
	benchmark.Arch = config.ArchType

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
