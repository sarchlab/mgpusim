// Package main runs the Parboil MRI-Q benchmark.
package main

import (
	"flag"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/parboil/mri_q"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var numVoxels = flag.Int("num_voxels", 65536, "Number of voxels")
var numK = flag.Int("num_k", 256, "Number of k-space samples")

func main() {
	flag.Parse()

	r := new(runner.Runner).Init()

	benchmark := mri_q.NewBenchmark(r.Driver())
	benchmark.NumVoxels = *numVoxels
	benchmark.NumK = *numK

	r.AddBenchmark(benchmark)
	r.Run()
}
