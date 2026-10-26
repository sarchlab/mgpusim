package main

import (
	"flag"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/memcopy"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
)

func run() timing.VTimeInPicoSec {
	c := sample.ParseConfig()
	s, r, finish := c.Build()
	defer finish()
	run := runner.New(s, r)
	b := memcopy.NewBenchmark(r)
	b.ByteSize = 1048576
	c.Configure(b)
	b.SetVerification(true)
	run.AddBenchmark(b)
	if err := s.Initialize(); err != nil {
		panic(err)
	}
	if err := run.Run(); err != nil {
		panic(err)
	}
	return s.Engine().CurrentTime()
}
func main() {
	flag.Parse()
	if run() != run() {
		panic("memory copy is not deterministic")
	}
}
