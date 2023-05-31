package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/minerva"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

var epochFlag = flag.Int("epoch", 1, "Number of epoch to run.")
var maxBatchPerEpochFlag = flag.Int("max-batch-per-epoch", 2,
	"Number of epochs to run.")
var batchSizeFlag = flag.Int("batch-size", 32,
	"Number of images per batch")
var enableTestingFlag = flag.Bool("enable-testing", false,
	"If set, the trainer will evaluate the trained model after each epoch")
var enableVerification = flag.Bool("enable-verification", false,
	`If set, all tenser operations will be verified against CPU results. Do not 
turn on if you care about the final results. This flag will introduce extra
GPU-to-CPU memory copies.`)

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := minerva.NewBenchmark(runner.Driver())
	benchmark.Epoch = *epochFlag
	benchmark.MaxBatchPerEpoch = *maxBatchPerEpochFlag
	benchmark.BatchSize = *batchSizeFlag
	benchmark.EnableTesting = *enableTestingFlag
	benchmark.EnableVerification = *enableVerification

	runner.AddBenchmark(benchmark)

	runner.Run()
}
