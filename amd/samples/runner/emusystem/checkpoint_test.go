package emusystem_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

const checkpointLength = 256

func TestEmulationCheckpointFreshProcess(t *testing.T) {
	if path := os.Getenv("MGPUSIM_AMD_EMU_RESTORE"); path != "" {
		a, err := strconv.Atoi(os.Getenv("MGPUSIM_AMD_ARCH"))
		if err != nil {
			t.Fatal(err)
		}
		s, d := checkpointSystem(t, arch.Type(a))
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		buffers := d.ResultBuffers()
		if len(buffers) != 2 {
			t.Fatalf("got %d unbound results", len(buffers))
		}
		output := [][]float32{make([]float32, checkpointLength), make([]float32, checkpointLength)}
		if err := d.BindResultBuffer(buffers[0].CommandID, output[0]); err != nil {
			t.Fatal(err)
		}
		// Let the second copy finish without a host buffer, then attach it.
		if err := s.Engine().Run(); err != nil {
			t.Fatal(err)
		}
		remaining := d.ResultBuffers()
		if len(remaining) != 1 || !remaining[0].Complete || remaining[0].CommandID != buffers[1].CommandID {
			t.Fatalf("missing completed unbound result: %+v", remaining)
		}
		if err := d.BindResultBuffer(buffers[1].CommandID, output[1]); err != nil {
			t.Fatal(err)
		}
		finishEmulation(t, s, output, path+".restored")
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
	s, d := checkpointSystem(t, a)
	output := submitFIR(t, d, a)
	d.TickLater()
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
	for _, v := range output[0] {
		if v != 0 {
			t.Fatal("host copy completed before checkpoint")
		}
	}
	path := filepath.Join(t.TempDir(), "mid-kernel.tar.gz")
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	finishEmulation(t, s, output, path+".uninterrupted")
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

func checkpointSystem(t *testing.T, a arch.Type) (*sim.Simulation, *driver.Driver) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	d := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).WithArchitecture(a).Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return s, d
}

func submitFIR(t *testing.T, d *driver.Driver, a arch.Type) [][]float32 {
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
	d.EnqueueMemCopyH2D(q, inPtr, input)
	d.EnqueueMemCopyH2D(q, filterPtr, taps)
	d.EnqueueMemCopyH2D(q, historyPtr, history)
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
	var args any = &fir.GCN3KernelArgs{Output: outPtr, Input: inPtr, Filter: filterPtr, History: historyPtr, NumTaps: 4}
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
	d.EnqueueMemCopyD2H(q, out, outPtr)
	second := make([]float32, checkpointLength)
	d.EnqueueLaunchKernel(q, code, [3]uint32{checkpointLength, 1, 1}, [3]uint16{64, 1, 1}, args)
	d.EnqueueMemCopyD2H(q, second, outPtr)
	return [][]float32{out, second}
}

func finishEmulation(t *testing.T, s *sim.Simulation, outputs [][]float32, path string) {
	t.Helper()
	if err := s.Engine().Run(); err != nil {
		t.Fatal(err)
	}
	for _, out := range outputs {
		verifyFIR(t, out)
	}
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified %d FIR outputs at %d ps", len(outputs)*checkpointLength, s.Engine().CurrentTime())
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
