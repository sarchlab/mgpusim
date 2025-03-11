package benchmark_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
)

func TestBenchmarkBuild(t *testing.T) {
	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("../data/simple-trace-example").
		Build()

	kernelCount := 0
	memcpyCount := 0

	for _, exec := range benchmark.TraceExecs {
		if exec.ExecType() == nvidia.ExecKernel {
			kernelCount++
		} else if exec.ExecType() == nvidia.ExecMemcpy {
			memcpyCount++
		} else {
			t.Errorf("Unknown exec type")
		}
	}

	// if kernelCount != 16 {
	// 	t.Errorf("Expected 16 kernel, got %d", kernelCount)
	// }
	// if memcpyCount != 14 {
	// 	t.Errorf("Expected 14 memcpy, got %d", memcpyCount)
	// }

	if kernelCount != 3 {
		t.Errorf("Expected 3 kernel, got %d", kernelCount)
	}
	if memcpyCount != 3 {
		t.Errorf("Expected 3 memcpy, got %d", memcpyCount)
	}
}
