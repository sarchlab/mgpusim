package benchmark

import (
	"testing"

	"github.com/sarchlab/accelsimtracing/nvidia"
)

func TestBenchmarkBuild(t *testing.T) {
	benchmark := new(BenchmarkBuilder).
		WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		Build()

	kernelCount := 0
	memcpyCount := 0

	for _, exec := range benchmark.TraceExecs() {
		if exec.ExecType() == nvidia.ExecKernel {
			kernelCount++
		} else if exec.ExecType() == nvidia.ExecMemcpy {
			memcpyCount++
		} else {
			t.Errorf("Unknown exec type")
		}
	}

	if kernelCount != 16 {
		t.Errorf("Expected 16 kernel, got %d", kernelCount)
	}
	if memcpyCount != 14 {
		t.Errorf("Expected 14 memcpy, got %d", memcpyCount)
	}
}
