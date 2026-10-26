package vgg16

import (
	"bytes"
	"flag"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputraining"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

type graphProbe struct {
	Result driver.CommandID
	Event  driver.Event
	Values []float32
	Time   timing.VTimeInPicoSec
}

func (*graphProbe) Name() string { return "VGGProbe" }
func (p *graphProbe) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, "probe", p, nil)
}
func (p *graphProbe) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, "probe", p, nil)
}

func graphSystem(
	t *testing.T, root string,
) (*sim.Simulation, *driver.Runtime, *runner.Runner, *Benchmark, *graphProbe) {
	t.Helper()
	old := flag.Lookup("imagenet-data-folder").Value.String()
	if err := flag.Set("imagenet-data-folder", root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := flag.Set("imagenet-data-folder", old); err != nil {
			t.Error(err)
		}
	})
	s := sim.MakeBuilder().WithTaskTracking().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	r := runner.New(
		s,
		rt,
	)
	b := NewBenchmark(rt)
	b.Epoch = 0
	b.BatchSize = 1
	b.MaxBatchPerEpoch = 1
	b.SelectGPU([]int{1})
	r.AddBenchmark(b)
	p := &graphProbe{}
	s.RegisterResource(p)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return s, rt, r, b, p
}
func makeImageFixture(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "train", "class", "images")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wnids.txt"), []byte("class\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "class_0.JPEG")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, image.NewGray(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func prepareGraph(t *testing.T, s *sim.Simulation, rt *driver.Runtime, b *Benchmark, p *graphProbe) {
	t.Helper()
	// Exercise the actual full VGG layer/parameter/view constructor. Seed the
	// trainer at epoch completion to bound this graph test; do not randomize the
	// >50-million-element FC matrix or claim full VGG training coverage.
	if err := b.init(); err != nil {
		t.Fatal(err)
	}
	b.Phase = 1
	state := b.trainer.State
	state.Phase = "epoch"
	var err error
	b.trainer, err = gputraining.Bind(rt, state)
	if err != nil {
		t.Fatal(err)
	}
	b.to = b.trainer.TensorOperators
	b.networks = b.trainer.Networks
	b.contexts = b.trainer.Contexts
	if len(b.networks) != 1 || len(b.networks[0].Layers) != 36 {
		t.Fatalf(
			"VGG graph has %d layers",
			len(b.networks[0].Layers),
		)
	}
	// Use its real ImageNet source and operator: submit one small image and an
	// explicit first-layer parameter transfer. All other model allocations remain
	// untouched sparse storage, keeping the archive bounded.
	input, labels := b.trainer.DataSource[0].NextBatch(1)
	if input.NumElement() != 3*224*224 || len(labels) != 1 {
		t.Fatal("ImageNet fixture shape")
	}
	params := b.networks[0].Layers[0].Parameters()
	values := make([]float64, params.NumElement())
	for i := range values {
		values[i] = float64(i%17) / 16
	}
	o := b.to[0]
	o.Init(params, values)
	p.Values = make(
		[]float32,
		len(values),
	)
	p.Result = rt.EnqueueMemCopyD2H(
		o.Queue(),
		params.(*gputensor.Tensor).Ptr(),
		len(values)*4,
	)
	p.Event = rt.EnqueueRecordEvent(o.Queue())
	for _, c := range s.Components() {
		if c.Name() == "Driver" {
			c.(interface{ TickLater() }).TickLater()
		}
	}
}
func finishGraph(
	t *testing.T,
	s *sim.Simulation,
	rt *driver.Runtime,
	r *runner.Runner,
	b *Benchmark,
	p *graphProbe,
	path string,
) {
	t.Helper()
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReadResult(p.Result, p.Values); err != nil {
		t.Fatal(err)
	}
	for i, v := range p.Values {
		if v != float32(i%17)/16 {
			t.Fatalf("first-layer parameter %d: %g", i, v)
		}
	}
	var err error
	p.Time, err = rt.EventTime(p.Event)
	if err != nil {
		t.Fatal(err)
	}
	state := b.trainer.State
	if !b.IsEnded() || state.Networks[0].Source.Images.Cursor != 1 {
		t.Fatal("wrapper completion or dataset cursor changed")
	}
	if err := r.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"full VGG graph restored; %d first-layer values, cursor1, marker%dps, end%dps",
		len(p.Values),
		p.Time,
		s.Engine().CurrentTime(),
	)
}

//nolint:funlen,gocognit // Keep the bounded fresh-process graph fixture and comparisons together.
func TestVGGGraphFreshProcessRoundTrip(t *testing.T) {
	if path := os.Getenv("MGPUSIM_VGG_GRAPH"); path != "" {
		s, rt, r, b, p := graphSystem(t, os.Getenv("MGPUSIM_VGG_ROOT"))
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishGraph(t, s, rt, r, b, p, path+".restored")
		return
	}
	root := t.TempDir()
	imagePath := makeImageFixture(t, root)
	path := filepath.Join(root, "graph.tar.gz")
	if !t.Run("uninterrupted", func(t *testing.T) {
		s, rt, r, b, p := graphSystem(t, root)
		prepareGraph(t, s, rt, b, p)
		if err := r.SaveCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(imagePath); err != nil {
			t.Fatal(err)
		}
		finishGraph(t, s, rt, r, b, p, path+".uninterrupted")
	}) {
		return
	}
	debug.FreeOSMemory()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestVGGGraphFreshProcessRoundTrip$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_VGG_GRAPH="+path, "MGPUSIM_VGG_ROOT="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restore: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
	expected, err := os.ReadFile(path + ".uninterrupted")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path + ".restored")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, actual) {
		t.Fatal("VGG graph/resource/continuation final archives differ")
	}
}
