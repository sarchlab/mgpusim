package driver

import (
	"errors"

	trace "github.com/sarchlab/mgpusim/v3/accelsim_tracing/trace"
)

type BenchmarkBuilder struct {
	fromTrace    bool
	traceDirPath string
}

func NewBenchmarkBuilder() *BenchmarkBuilder {
	return &BenchmarkBuilder{
		fromTrace:    false,
		traceDirPath: "",
	}
}

func (b *BenchmarkBuilder) WithTraceDirPath(path string) *BenchmarkBuilder {
	b.traceDirPath = path
	b.fromTrace = true
	return b
}

func (b *BenchmarkBuilder) Build() (*Benchmark, error) {
	if !b.fromTrace {
		return nil, errors.New("no trace dir path specified")
	}

	bm := &Benchmark{
		traceParser: trace.NewTraceParser(b.traceDirPath),
		TraceExecs:  nil,
	}
	
	bm.TraceExecs = bm.traceParser.BuildTraceExecutions()
	return bm, nil
}
