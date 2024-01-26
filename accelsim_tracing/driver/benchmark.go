package driver

import (
	trace "github.com/sarchlab/mgpusim/v3/accelsim_tracing/trace"
)

type Benchmark struct {
	traceParser *trace.TraceParser
	TraceExecs  *[]trace.TraceExecs
}
