package benchmark_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

func TestBenchmarkBuild(t *testing.T) {
	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("../data/simple-trace-example").
		Build()

	kernelCount := 0
	memcpyCount := 0

	for _, exec := range benchmark.TraceExecs {
		if exec.ExecType() == trace.ExecKernel {
			kernelCount++
		} else if exec.ExecType() == trace.ExecMemcpy {
			memcpyCount++
		} else {
			t.Errorf("Unknown exec type")
		}
	}

	if kernelCount != 1 {
		t.Errorf("Expected 1 kernel, got %d", kernelCount)
	}
	if memcpyCount != 2 {
		t.Errorf("Expected 2 memcpy, got %d", memcpyCount)
	}
}
