package benchmark

import (
	"errors"

	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/gpu"
	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/trace"
)

type BenchMark struct {
	fromTrace    bool
	traceDirPath string
	trace        *trace.Trace
}

func NewBenchMark() *BenchMark {
	return &BenchMark{
		fromTrace:    false,
		traceDirPath: "",
		trace:        nil,
	}
}

func (bm *BenchMark) WithTraceDirPath(path string) *BenchMark {
	bm.traceDirPath = path
	bm.fromTrace = true
	return bm
}

func (bm *BenchMark) Build() error {
	if bm.fromTrace == false {
		return errors.New("no trace dir path specified")
	}
	bm.trace = trace.NewTrace().WithTraceDirPath(bm.traceDirPath)
	bm.trace.Build()
	return nil
}

func (bm *BenchMark) Exec(gpu *gpu.GPU) error {
	if bm.fromTrace == false {
		panic("No trace dir path specified")
	}
	err := bm.trace.Exec(gpu)
	return err
}
