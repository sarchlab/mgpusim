package timingconfig

import (
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/lbm"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/parboil/sgemm"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
)

// With one CU per die these grids put enough waves on each SIMD to use
// both halves of its VGPR file. Aliasing adjacent lanes corrupts SGEMM's
// LDS addresses and LBM's global memory accesses.
func TestMI300XVGPRLaneIsolation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*gpuruntime.Runtime) benchmarks.Benchmark
	}{
		{"SGEMM", func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
			b := sgemm.NewBenchmark(rt)
			b.N, b.Arch = 112, arch.CDNA3
			return b
		}},
		{"LBM", func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
			b := lbm.NewBenchmark(rt)
			b.GridDim, b.NumTimesteps, b.Arch = 16, 1, arch.CDNA3
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := buildStandaloneGPU(t, "mi300x", 21, false, func(b gpubuilder.GPUBuilder) gpubuilder.GPUBuilder {
				return b.(mi300x.Builder).WithNumShaderArray(mi300x.NumXCD).WithNumCUPerShaderArray(1)
			})
			b := tc.build(p.rt)
			b.SelectGPU([]int{1})
			b.SetVerification(true)
			r := runner.New(p.sim, p.rt)
			r.AddBenchmark(b)
			if err := p.sim.Initialize(); err != nil {
				t.Fatal(err)
			}
			if err := r.Run(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
