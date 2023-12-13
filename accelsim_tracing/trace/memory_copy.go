package trace

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"

type memCopy struct { // trace execs interface
	rawText   string
	h2d       bool
	startAddr uint64
	length    uint64
}

func (te *memCopy) Type() string {
	return "memcopy"
}

func (te *memCopy) File() string {
	return ""
}

func (te *memCopy) Exec(g *gpu.GPU) error {
	// [todo] implement
	return nil
}
