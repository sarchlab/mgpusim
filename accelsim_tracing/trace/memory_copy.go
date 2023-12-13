package trace

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"

type memCopy struct { // trace execs interface
	parent *Trace

	rawText   string
	h2d       bool
	startAddr uint64
	length    uint64
}

type memCopyParent struct {
	trace *Trace
}

func (te *memCopy) Type() string {
	return "memcopy"
}

func (te *memCopy) Execute(gpu *gpu.GPU) error {
	return nil
}
