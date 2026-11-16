package runner_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// A fresh process only rebuilds the topology. It does not read the trace or
// resubmit kernels; the archive must contain all submitted work and progress.
func TestCheckpointFreshProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("fresh-process checkpoint test builds full simulations; run without -short")
	}
	if path := os.Getenv("MGPUSIM_NVIDIA_RESTORE"); path != "" {
		s, p := checkpointPlatform(t)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishCheckpointRun(t, s, p, path+".restored")
		return
	}
	for _, phase := range []string{"queued", "resident", "memory"} {
		t.Run(phase, func(t *testing.T) {
			runCheckpointFreshProcess(t, phase)
		})
	}
}

func runCheckpointFreshProcess(t *testing.T, phase string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mid-kernel.tar.gz")
	// End the parent run and its cleanup scope before building a second
	// simulation in the restore process. No simulation escapes this subtest.
	if !t.Run("uninterrupted", func(t *testing.T) {
		prepareCheckpoint(t, phase, path)
	}) {
		return
	}
	// Return the now-unreachable parent heap to the OS before starting the child.
	debug.FreeOSMemory()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCheckpointFreshProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_NVIDIA_RESTORE="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restore process: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
	want, err := os.ReadFile(path + ".uninterrupted")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path + ".restored")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("restored final checkpoint differs from uninterrupted execution")
	}
	t.Logf("%s: complete final archives match, including engine time, IDs, memory and component state", phase)
}

func prepareCheckpoint(t *testing.T, phase string, path string) {
	t.Helper()
	s, p := checkpointPlatform(t)
	for range 2 {
		for _, work := range benchmark.Load("../trace/testdata/vectoradd").TraceExecs {
			work.Run(p.Driver)
		}
	}
	p.Driver.TickLater()
	engine := s.Engine().(*timing.SerialEngine)
	reached := phase == "queued"
	for cycles := 0; !reached && cycles < 10000; cycles++ {
		if err := engine.RunUntil(engine.CurrentTime() + 1000); err != nil {
			t.Fatal(err)
		}
		resident, memory := 0, 0
		for _, g := range p.Devices {
			for _, sm := range g.SMList {
				for _, sp := range sm.SMSPs {
					w, _, m := sp.Status()
					resident += w
					memory += m
				}
			}
		}
		reached = (phase == "resident" && resident > 0) || (phase == "memory" && memory > 0)
	}
	if !reached {
		t.Fatalf("never reached %s", phase)
	}
	if p.Driver.UnfinishedKernels() != 2 {
		t.Fatal("checkpoint must retain active and queued work")
	}
	assertTraceCount(t, p, 2)
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	finishCheckpointRun(t, s, p, path+".uninterrupted")
	rewindCheckpoint(t, s, p, path)
}

func rewindCheckpoint(t *testing.T, s *sim.Simulation, p *platform.Platform, path string) {
	t.Helper()
	// Restore the same component instances twice after they have completed all work.
	// This catches lookups or observers that retain pre-restore State references.
	for range 2 {
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishCheckpointRun(t, s, p, path+".rewound")
		want, err := os.ReadFile(path + ".uninterrupted")
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path + ".rewound")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Fatal("same-component rewind changed final archive")
		}
	}
}

func checkpointPlatform(t *testing.T) (*sim.Simulation, *platform.Platform) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	device := platform.H100()
	device.Freq = timing.GHz
	device.LaunchOverheadLatency = 5
	device.GPU.NumSMs = 2
	device.GPU.NumSMSPsPerSM = 2
	device.GPU.NumMemoryBanks = 2
	device.GPU.L2CacheSize = 32 * 1024
	p := platform.Build(s, device)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	return s, p
}

func finishCheckpointRun(t *testing.T, s *sim.Simulation, p *platform.Platform, path string) {
	t.Helper()
	if err := s.Engine().Run(); err != nil {
		t.Fatal(err)
	}
	assertTraceCount(t, p, 0)
	if p.Driver.UnfinishedKernels() != 0 {
		t.Fatal("submitted kernels did not finish")
	}
	var insts, warps uint64
	for _, g := range p.Devices {
		for _, sm := range g.SMList {
			warps += sm.GetTotalWarpsCount()
			for _, sp := range sm.SMSPs {
				insts += sp.GetTotalInstsCount()
			}
		}
	}
	if insts != 544 || warps != 32 {
		t.Fatalf("wrong completed work: %d instructions, %d warps", insts, warps)
	}
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("completed 2 kernels, %d instructions, %d warps at %d ps", insts, warps, p.Driver.FinishTime()))
}

//nolint:musttag // Inspect the persisted resource envelope with its canonical checkpoint field names.
func assertTraceCount(t *testing.T, p *platform.Platform, want int) {
	t.Helper()
	var data bytes.Buffer
	if err := p.Driver.Resources.Traces.SaveCheckpoint(&data); err != nil {
		t.Fatal(err)
	}
	var saved struct {
		State struct {
			Kernels map[string]json.RawMessage `json:"Kernels"`
		}
	}
	if err := json.Unmarshal(data.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.State.Kernels) != want {
		t.Fatalf("trace ownership: got %d, want %d", len(saved.State.Kernels), want)
	}
}

func TestEmptyKernelReleasesTrace(t *testing.T) {
	s, p := checkpointPlatform(t)
	p.Driver.RunKernel(&trace.KernelTrace{ID: "empty"})
	assertTraceCount(t, p, 1)
	p.Driver.TickLater()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if p.Driver.UnfinishedKernels() != 0 {
		t.Fatal("empty kernel did not complete")
	}
	assertTraceCount(t, p, 0)
}
