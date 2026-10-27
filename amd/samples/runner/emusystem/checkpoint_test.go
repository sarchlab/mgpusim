package emusystem_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

const checkpointLength = 256

func TestEmulationCheckpointFreshProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("fresh-process checkpoint test builds full simulations; run without -short")
	}
	if path := os.Getenv("MGPUSIM_AMD_EMU_RESTORE"); path != "" {
		a, err := strconv.Atoi(os.Getenv("MGPUSIM_AMD_ARCH"))
		if err != nil {
			t.Fatal(err)
		}
		s, d, results := checkpointSystem(t, arch.Type(a))
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		if len(results.IDs) != 2 {
			t.Fatalf("expected two saved readbacks: %v", results.IDs)
		}
		// Host reads deliberately do not guard pending destinations. The
		// saved mid-kernel destination must still hold its initial zero bytes.
		output := make([]uint32, results.IDs[0].ByteSize/4)
		if err := d.ReadHost(results.IDs[0], output); err != nil {
			t.Fatal(err)
		}
		for _, v := range output {
			if v != 0 {
				t.Fatal("pending destination changed before copy")
			}
		}

		finishEmulation(t, s, d, results, path+".restored")
		return
	}
	for _, a := range []arch.Type{arch.GCN3, arch.CDNA3} {
		t.Run(a.String(), func(t *testing.T) {
			runEmulationCheckpointFreshProcess(t, a)
		})
	}
}

func runEmulationCheckpointFreshProcess(t *testing.T, a arch.Type) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mid-kernel.tar.gz")
	// End the parent run and its cleanup scope before building a second
	// simulation in the restore process. No simulation escapes this subtest.
	if !t.Run("uninterrupted", func(t *testing.T) {
		prepareEmulationCheckpoint(t, a, path)
	}) {
		return
	}
	// Return the now-unreachable parent heap to the OS before starting the child.
	debug.FreeOSMemory()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestEmulationCheckpointFreshProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_AMD_EMU_RESTORE="+path, "MGPUSIM_AMD_ARCH="+strconv.Itoa(int(a)))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restore child: %v\n%s", err, out)
	} else {
		t.Logf("%s", out)
	}
	want, err := os.ReadFile(path + ".uninterrupted")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path + ".restored")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("final restored archive differs from uninterrupted run")
	}
	t.Logf("%s: FIR output verified; full final checkpoints match", a)
}

func prepareEmulationCheckpoint(t *testing.T, a arch.Type, path string) {
	t.Helper()
	s, d, results := checkpointSystem(t, a)
	results.IDs = submitFIR(t, d, a)
	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			d.TickLater()
		}
	}
	e := s.Engine().(*timing.SerialEngine)
	reached := false
	for cycle := 0; cycle < 5000; cycle++ {
		if err := e.RunUntil(timing.VTimeInPicoSec(cycle+1) * 1000); err != nil {
			t.Fatal(err)
		}
		for _, c := range s.Components() {
			if cu, ok := c.(*emu.Comp); ok && len(cu.State.QueuedWGs) > 0 {
				if err := e.RunUntil(cu.State.NextEmulationAt); err != nil {
					t.Fatal(err)
				}
				reached = len(cu.State.PendingCompletions) > 0
			}
		}
		if reached {
			break
		}
	}
	if !reached {
		t.Fatal("never reached an in-flight work-group")
	}
	out := make([]float32, checkpointLength)
	if err := d.ReadHost(results.IDs[0], out); err != nil {
		t.Fatal(err)
	}
	for _, v := range out {
		if v != 0 {
			t.Fatal("destination changed before queued D2H")
		}
	}
	if d.Unfinished()[0] == 0 {
		t.Fatal("copy pipeline completed before checkpoint")
	}

	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	finishEmulation(t, s, d, results, path+".uninterrupted")
	rewindEmulation(t, s, d, results, path)
}

func checkpointSystem(t *testing.T, a arch.Type) (*sim.Simulation, *gpuruntime.Runtime, *firResults) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	d := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithArchitecture(a).Build()
	results := &firResults{}
	s.RegisterResource(results)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return s, d, results
}

func submitFIR(t *testing.T, d *gpuruntime.Runtime, a arch.Type) []gpuruntime.HostBuffer {
	t.Helper()
	ctx := d.Init()
	q := d.CreateCommandQueue(ctx)
	input := make([]float32, checkpointLength)
	for i := range input {
		input[i] = float32(i)
	}
	taps := []float32{0, 1, 2, 3}
	history := make([]float32, len(taps))
	out := make([]float32, checkpointLength)
	inPtr := d.AllocateMemory(ctx, uint64(len(input)*4))
	outPtr := d.AllocateMemory(ctx, uint64(len(out)*4))
	filterPtr := d.AllocateMemory(ctx, uint64(len(taps)*4))
	historyPtr := d.AllocateMemory(ctx, uint64(len(history)*4))
	benchmarks.Upload(d, q, inPtr, input)
	benchmarks.Upload(d, q, filterPtr, taps)
	benchmarks.Upload(d, q, historyPtr, history)
	filename := "kernels.hsaco"
	if a == arch.CDNA3 {
		filename = "kernels_gfx942.hsaco"
	}
	data, err := os.ReadFile(filepath.Join("../../../benchmarks/heteromark/fir", filename))
	if err != nil {
		t.Fatal(err)
	}
	code := insts.LoadKernelCodeObjectFromBytes(data, "FIR")
	if code == nil {
		t.Fatal("missing FIR kernel")
	}
	var args any = &fir.GCN3KernelArgs{
		Output:  outPtr,
		Input:   inPtr,
		Filter:  filterPtr,
		History: historyPtr,
		NumTaps: 4,
	}
	if a == arch.CDNA3 {
		args = &fir.CDNA3KernelArgs{
			Output:            outPtr,
			Input:             inPtr,
			Filter:            filterPtr,
			History:           historyPtr,
			NumTaps:           4,
			HiddenBlockCountX: checkpointLength / 64,
			HiddenBlockCountY: 1,
			HiddenBlockCountZ: 1,
			HiddenGroupSizeX:  64,
			HiddenGroupSizeY:  1,
			HiddenGroupSizeZ:  1,
		}
	}
	d.EnqueueLaunchKernel(q, code, [3]uint32{checkpointLength, 1, 1}, [3]uint16{64, 1, 1}, args)
	first := benchmarks.Download(d, q, outPtr, len(out)*4)

	d.EnqueueLaunchKernel(q, code, [3]uint32{checkpointLength, 1, 1}, [3]uint16{64, 1, 1}, args)
	second := benchmarks.Download(d, q, outPtr, checkpointLength*4)
	return []gpuruntime.HostBuffer{first, second}
}

func finishEmulation(t *testing.T, s *sim.Simulation, d *gpuruntime.Runtime, results *firResults, path string) {
	t.Helper()
	if err := s.Engine().Run(); err != nil {
		t.Fatal(err)
	}
	for _, id := range results.IDs {
		out := make([]float32, checkpointLength)
		if err := benchmarks.Read(d, id, out); err != nil {
			t.Fatal(err)
		}
		verifyFIR(t, out)
		results.Outputs = append(results.Outputs, out)
	}
	results.IDs = nil
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified %d FIR outputs at %d ps", len(results.Outputs)*checkpointLength, s.Engine().CurrentTime())
}

func verifyFIR(t *testing.T, out []float32) {
	t.Helper()
	for i, v := range out {
		var want float32
		for tap := 0; tap < 4; tap++ {
			if i >= tap {
				want += float32(i-tap) * float32(tap)
			}
		}
		if v != want {
			t.Fatalf("FIR[%d]=%v, want %v", i, v, want)
		}
	}
}

type firResults struct {
	IDs     []gpuruntime.HostBuffer
	Outputs [][]float32
}

func (r *firResults) Name() string { return "FIRResults" }
func (r *firResults) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, checkpointLength, r, nil)
}
func (r *firResults) LoadCheckpoint(reader io.Reader) error {
	return modeling.ReadCheckpoint(reader, checkpointLength, r, nil)
}

func rewindEmulation(t *testing.T, s *sim.Simulation, d *gpuruntime.Runtime, results *firResults, path string) {
	t.Helper()
	// Rewind completed component instances, including their caches, to the same
	// in-flight IDs twice. No setup or lifecycle hook runs during a restore.
	for range 2 {
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishEmulation(t, s, d, results, path+".rewound")
		want, err := os.ReadFile(path + ".uninterrupted")
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path + ".rewound")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Fatal("same-instance rewind changed final checkpoint")
		}
	}
}
