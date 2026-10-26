package main

import (
	"flag"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training_benchmarks/lenet"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

var epochFlag = flag.Int("epoch", 1, "Number of epoch to run.")
var maxBatchPerEpochFlag = flag.Int("max-batch-per-epoch", 2,
	"Number of epochs to run.")
var batchSizeFlag = flag.Int("batch-size", 32,
	"Number of images per batch")
var enableTestingFlag = flag.Bool("enable-testing", false,
	"If enable testing is set, the trainer will evaluate the trained model after each epoch")
var enableVerification = flag.Bool("enable-verification", false,
	"Verify tensor operations against CPU results (also enabled by -verify). Adds verification copies.")

func main() {
	rand.Seed(1)
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	runner := runner.New(simulation, runtime)

	benchmark := lenet.NewBenchmark(runtime)
	benchmark.Epoch = *epochFlag
	benchmark.MaxBatchPerEpoch = *maxBatchPerEpochFlag
	benchmark.BatchSize = *batchSizeFlag
	benchmark.EnableTesting = *enableTestingFlag
	config.Verify = config.Verify || *enableVerification
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
