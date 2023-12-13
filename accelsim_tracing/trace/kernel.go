package trace

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type kernel struct { // trace execs interface
	rawText    string
	fileName   string
	filePath   string
	traceGroup *traceGroup
}

func (te *kernel) Type() string {
	return "kernel"
}

func (te *kernel) Exec(gpu *gpu.GPU) error {
	tg := NewTraceGroup().WithFilePath(te.filePath)
	tg.Build()

	err := tg.Exec(gpu)

	return err
}
