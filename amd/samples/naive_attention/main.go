package main

import (
	"flag"

	naiveattention "github.com/sarchlab/mgpusim/v4/amd/benchmarks/microbench/naive_attention"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var batch = flag.Int("batch", 4, "Batch size")
var seqLen = flag.Int("seq_len", 128, "Sequence length")
var numHeads = flag.Int("num_heads", 12, "Number of heads")
var headDim = flag.Int("head_dim", 64, "Head dimension")
var maxBatchHeads = flag.Int("max_batch_heads", 0,
	"Max batch-head groups to launch (0 = batch*num_heads)")
var maxQueryRows = flag.Int("max_query_rows", 0,
	"Max query rows to launch (0 = seq_len)")

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := naiveattention.NewBenchmark(runner.Driver())
	benchmark.SetBatch(*batch)
	benchmark.SetSeqLen(*seqLen)
	benchmark.SetNumHeads(*numHeads)
	benchmark.SetHeadDim(*headDim)
	benchmark.SetMaxBatchHeads(*maxBatchHeads)
	benchmark.SetMaxQueryRows(*maxQueryRows)

	runner.AddBenchmark(benchmark)

	runner.Run()
}
