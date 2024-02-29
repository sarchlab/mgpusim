package driver

import (
	"errors"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/trace"
)

type BenchmarkBuilder struct {
	fromTrace    bool // [not] from other ?
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

func (b *BenchmarkBuilder) Build() (*nvidia.KernelList, error) {
	if !b.fromTrace {
		return nil, errors.New("no trace dir path specified")
	}

	klReader := trace.NewKLReader(b.traceDirPath)
	kl := klReader.Read()

	return kl, nil
}
