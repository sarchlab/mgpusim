package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/rope"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var batch = flag.Int("batch", 4, "Batch size")
var seqLen = flag.Int("seq_len", 128, "Sequence length")
var numHeads = flag.Int("num_heads", 32, "Number of heads")
var headDim = flag.Int("head_dim", 128, "Head dimension")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := rope.NewBenchmark(runner.Driver())
	benchmark.SetBatch(*batch)
	benchmark.SetSeqLen(*seqLen)
	benchmark.SetNumHeads(*numHeads)
	benchmark.SetHeadDim(*headDim)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
