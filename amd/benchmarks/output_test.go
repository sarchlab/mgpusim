package benchmarks_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/bitonicsort"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/fastwalshtransform"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/floydwarshall"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/matrixmultiplication"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/matrixtranspose"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/nbody"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/simpleconvolution"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/amdappsdk/vectoradd"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layer_benchmarks/relu"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/pagerank"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/atax"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/polybench/bicg"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/rodinia/nw"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/bfs"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/fft"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/spmv"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/shoc/stencil2d"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type outputCase struct {
	name         string
	outputs      []string
	newBenchmark func(*gpuruntime.Runtime) benchmarks.Benchmark
}
type outputCopies struct {
	tasks.NopObserver
	count int
}

func (o *outputCopies) TaskEnded(task tasks.Task) {
	if task.What == "*driver.MemCopyD2HCommand" {
		o.count++
	}
}
func savedState(t *testing.T, resource interface{ SaveCheckpoint(io.Writer) error }) map[string]json.RawMessage {
	t.Helper()
	var buf bytes.Buffer
	if err := resource.SaveCheckpoint(&buf); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		State map[string]json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.State
}

// Baseline Run/exec always materialized these host outputs. Verification may
// suppress CPU comparisons, but must not remove their simulated transfer or
// leave output CPU buffers unprocessed.
//
//nolint:funlen // Keep the complete baseline output-copy audit table together.
func normalOutputCases() []outputCase {
	return []outputCase{
		{name: "relu", outputs: []string{"OutputData"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := relu.NewBenchmark(rt)
				b.Length = 64
				return b
			}},
		{name: "vectoradd", outputs: []string{"HA"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := vectoradd.NewBenchmark(rt)
				b.Width, b.Height = 64, 1
				return b
			}},
		{name: "nbody", outputs: []string{"Pos"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := nbody.NewBenchmark(rt)
				b.NumParticles, b.NumIterations = 64, 1
				return b
			}},
		{name: "spmv", outputs: []string{"Out"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := spmv.NewBenchmark(rt)
				b.Dim, b.Sparsity = 64, 0.125
				return b
			}},
		{name: "stencil2d", outputs: []string{"HOutput"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := stencil2d.NewBenchmark(rt)
				b.NumRows, b.NumCols, b.NumIteration = 66, 66, 1
				return b
			}},
		{name: "fft", outputs: []string{"Result"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := fft.NewBenchmark(rt)
				b.Bytes, b.BytesMode, b.Passes = 8192, true, 1
				return b
			}},
		{name: "floydwarshall", outputs: []string{"HOutputPathMatrix", "HOutputPathDistanceMatrix"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := floydwarshall.NewBenchmark(rt)
				b.NumNodes = 16
				return b
			}},
		{name: "matrixtranspose", outputs: []string{"HOutputData"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := matrixtranspose.NewBenchmark(rt)
				b.Width = 64
				return b
			}},
		{name: "pagerank", outputs: []string{"HPageRank"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := pagerank.NewBenchmark(rt)
				b.NumNodes, b.NumConnections, b.MaxIterations = 16, 32, 2
				return b
			}},
		{name: "bitonicsort", outputs: []string{"OutputData"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := bitonicsort.NewBenchmark(rt)
				b.Length, b.OrderAscending = 64, true
				return b
			}},
		{name: "nw", outputs: []string{"OutputItemSets"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := nw.NewBenchmark(rt)
				b.SetLength(64)
				return b
			}},
		{name: "simpleconvolution", outputs: []string{"HOutputData"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := simpleconvolution.NewBenchmark(rt)
				b.Width, b.Height = 16, 16
				b.SetMaskSize(3)
				return b
			}},
		{name: "atax", outputs: []string{"YOutput"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := atax.NewBenchmark(rt)
				b.NX, b.NY = 16, 16
				return b
			}},
		{name: "bicg", outputs: []string{"SOutput", "QOutput"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := bicg.NewBenchmark(rt)
				b.NX, b.NY = 16, 16
				return b
			}},
		{name: "fastwalshtransform", outputs: []string{"HInputArray"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := fastwalshtransform.NewBenchmark(rt)
				b.Length = 64
				return b
			}},
		{name: "matrixmultiplication", outputs: []string{"MatrixC"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := matrixmultiplication.NewBenchmark(rt)
				b.X, b.Y, b.Z = 32, 32, 32
				return b
			}},
		{name: "bfs", outputs: []string{"HCost"},
			newBenchmark: func(rt *gpuruntime.Runtime) benchmarks.Benchmark {
				b := bfs.NewBenchmark(rt)
				b.NumNode, b.Degree, b.MaxDepth = 32, 3, 10
				return b
			}},
	}
}
func TestNormalOutputsSurviveDisabledVerification(t *testing.T) {
	for _, tc := range normalOutputCases() {
		t.Run(tc.name, func(t *testing.T) {
			want, copies := runOutputCase(t, tc, true)
			got, uncheckedCopies := runOutputCase(t, tc, false)
			if copies == 0 || uncheckedCopies != copies {
				t.Fatalf("output D2H count: verified %d, unchecked %d", copies, uncheckedCopies)
			}
			for _, field := range tc.outputs {
				if len(want[field]) == 0 || bytes.Equal(want[field], []byte("null")) {
					t.Fatalf("missing output %s", field)
				}
				if !bytes.Equal(want[field], got[field]) {
					t.Fatalf("verification-off changed host output %s", field)
				}
			}
		})
	}
}
func runOutputCase(t *testing.T, tc outputCase, verify bool) (map[string]json.RawMessage, int) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	b := tc.newBenchmark(rt)
	b.SelectGPU([]int{1})
	b.SetVerification(verify)
	r := runner.New(s, rt)
	r.AddBenchmark(b)
	copies := &outputCopies{}
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			s.TaskTracker().Subscribe(copies, c)
		}
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	}()
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(savedState(t, rt)["State"], &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["Results"]; ok {
		t.Fatal("Runtime retained a result mailbox")
	}
	return savedState(t, b), copies.count
}

// Each of these baseline single-queue workloads allocated and launched on the
// last selected GPU. CPU staging must not change that placement.
func TestSingleQueueWorkloadsKeepLastSelectedGPU(t *testing.T) {
	affected := map[string]bool{
		"atax": true, "bicg": true, "floydwarshall": true, "nbody": true,
		"pagerank": true, "spmv": true, "fft": true, "stencil2d": true,
	}
	for _, tc := range normalOutputCases() {
		if !affected[tc.name] {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(2).Build()
			b := tc.newBenchmark(rt)
			b.SelectGPU([]int{1, 2})
			if err := s.Initialize(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.Terminate(); err != nil {
					t.Error(err)
				}
			}()
			if err := b.Step(); err != nil {
				t.Fatal(err)
			}
			checkLastGPUPlacement(t, s, rt)
		})
	}
}

func checkLastGPUPlacement(t *testing.T, s *sim.Simulation, rt *gpuruntime.Runtime) {
	t.Helper()
	var state struct {
		Queues []struct {
			GPU int `json:"GPU"`
		} `json:"Queues"`
		Contexts []struct {
			PID     uint64 `json:"PID"`
			GPU     int    `json:"GPU"`
			Buffers []struct {
				Address gpuruntime.Ptr `json:"Address"`
			} `json:"Buffers"`
		} `json:"Contexts"`
	}
	if err := json.Unmarshal(savedState(t, rt)["State"], &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Queues) != 1 || state.Queues[0].GPU != 2 {
		t.Fatal("queue placement", state.Queues)
	}
	var component *driver.Comp
	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			component = d
		}
	}
	devicePages := 0
	for _, context := range state.Contexts {
		if context.GPU != 2 {
			t.Fatal("allocation selection changed", context.GPU)
		}
		for _, buffer := range context.Buffers {
			page, ok := component.Resources.PageTable.Find(vm.PID(context.PID), uint64(buffer.Address))
			if !ok {
				t.Fatal("allocation has no mapping")
			}
			if page.DeviceID == 0 {
				continue
			}
			devicePages++
			if page.DeviceID != 2 {
				t.Fatal("GPU allocation placement", page.DeviceID)
			}
		}
	}
	if devicePages == 0 {
		t.Fatal("fixture did not allocate GPU data")
	}
}
