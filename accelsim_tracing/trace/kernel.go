package trace

import (
	"path"

	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/gpu"
)

type kernel struct { // trace execs interface
	parent *Trace

	rawText    string
	filePath   string
	traceGroup *traceGroup
}

func (te *kernel) Type() string {
	return "kernel"
}

func (te *kernel) Execute(gpu *gpu.GPU) error {
	tg := NewTraceGroup().WithFilePath(path.Join(te.parent.traceDirPath, te.filePath))
	tg.Build()
	err := tg.Exec(gpu)
	return err
}
