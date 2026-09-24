package benchmark_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v5/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

func TestLoad(t *testing.T) {
	b := benchmark.Load("../trace/testdata/vectoradd")

	kernelCount := 0
	memcpyCount := 0

	for _, exec := range b.TraceExecs {
		switch exec.ExecType() {
		case trace.ExecKernel:
			kernelCount++
		case trace.ExecMemcpy:
			memcpyCount++
		default:
			t.Errorf("unknown exec type %v", exec.ExecType())
		}
	}

	if kernelCount != 1 {
		t.Errorf("expected 1 kernel, got %d", kernelCount)
	}

	if memcpyCount != 2 {
		t.Errorf("expected 2 memcpy, got %d", memcpyCount)
	}
}
