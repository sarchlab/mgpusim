package driver

import (
	"errors"

	trace "github.com/sarchlab/mgpusim/v3/accelsim_tracing/trace"
)

type Benchmark struct {
	fromTrace    bool
	traceDirPath string
	traceParser  *trace.TraceParser
	TraceExecs   *[]trace.TraceExecs
}

func NewBenchmark() *Benchmark {
	return &Benchmark{
		fromTrace:    false,
		traceDirPath: "",
	}
}

func (b *Benchmark) WithTraceDirPath(path string) *Benchmark {
	b.traceDirPath = path
	b.fromTrace = true
	return b
}

func (b *Benchmark) Build() error {
	if !b.fromTrace {
		return errors.New("no trace dir path specified")
	}
	b.traceParser = trace.NewTraceParser(b.traceDirPath)
	b.TraceExecs = b.traceParser.BuildTraceExecutions()
	return nil
}
