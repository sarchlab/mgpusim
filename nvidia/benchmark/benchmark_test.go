package benchmark_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
)

func TestBenchmarkBuild(t *testing.T) {
	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("../data/simple-trace-example").
		Build()

	kernelCount := 0
	memcpyCount := 0

	for _, exec := range benchmark.TraceExecs {
		if exec.ExecType() == nvidiaconfig.ExecKernel {
			kernelCount++
		} else if exec.ExecType() == nvidiaconfig.ExecMemcpy {
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

	// if kernelCount != 3 {
	// 	t.Errorf("Expected 3 kernel, got %d", kernelCount)
	// }
	// if memcpyCount != 3 {
	// 	t.Errorf("Expected 3 memcpy, got %d", memcpyCount)
	// }

	if kernelCount != 1 {
		t.Errorf("Expected 1 kernel, got %d", kernelCount)
	}
	if memcpyCount != 2 {
		t.Errorf("Expected 2 memcpy, got %d", memcpyCount)
	}
}
