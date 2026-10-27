package timingconfig_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A small FIR cannot train the sampler's 1024-wave warmup. Feed a deterministic
// history through its public API, then exercise real dispatch and completion.
// The first kernel is sampled (so its numerical output is deliberately skipped);
// the second resets the sampler and runs normally. The existing checkpoint test
// separately verifies full FIR numerical results on both architectures.
func TestSampledFIRCheckpointFreshProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a full simulation and restores it in a child process")
	}
	if path := os.Getenv("MGPUSIM_SAMPLED_RESTORE"); path != "" {
		s, _, _ := checkpointSystem(t, arch.GCN3)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishSampledFIR(t, s, path+".restored")
		return
	}
	path := filepath.Join(t.TempDir(), "sampled.tar.gz")
	if !t.Run("uninterrupted", func(t *testing.T) { prepareSampledFIR(t, path) }) {
		return
	}
	debug.FreeOSMemory()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSampledFIRCheckpointFreshProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_SAMPLED_RESTORE="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore child: %v\n%s", err, out)
	}
	t.Logf("%s", out)
	for _, suffix := range []string{".json", ".tar.gz"} {
		want, err := os.ReadFile(path + ".uninterrupted" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path + ".restored" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("sampled fresh-process result differs: %s", suffix)
		}
	}
}

func prepareSampledFIR(t *testing.T, path string) {
	t.Helper()
	s, runtime, results := checkpointSystem(t, arch.GCN3)
	results.IDs = submitFIR(t, runtime, arch.GCN3)
	for _, c := range s.Components() {
		if d, ok := c.(*driver.Comp); ok {
			d.TickLater()
		}
	}
	e := s.Engine().(*timing.SerialEngine)
	for cycle := 1; cycle <= 20000; cycle++ {
		if err := e.RunUntil(timing.VTimeInPicoSec(cycle) * 1000); err != nil {
			t.Fatal(err)
		}
		if len(pendingSampledTasks(s)) > 0 {
			if err := s.SaveCheckpoint(path); err != nil {
				t.Fatal(err)
			}
			finishSampledFIR(t, s, path+".uninterrupted")
			return
		}
		// The CP resets sampling on kernel admission. Re-prime only until the
		// first real dispatch uses it; no training is injected after the checkpoint.
		for _, c := range s.Components() {
			if comp, ok := c.(*cu.Comp); ok {
				primeSampler(t, comp.Resources.Sampler)
				break
			}
		}
	}
	t.Fatal("FIR never dispatched a sampled wave")
}

func primeSampler(t *testing.T, sampler *sampling.SampledEngine) {
	t.Helper()
	sampler.Enable()
	if _, enabled := sampler.Predict(); enabled {
		return
	}
	for i := 0; i < 4096; i++ {
		issue := timing.VTimeInPicoSec(i * 1000)
		sampler.Collect(issue, issue+10501)
	}
	if _, enabled := sampler.Predict(); !enabled {
		t.Fatal("controlled sampling history did not stabilize")
	}
}

type pendingSample struct {
	Location string
	Wave     uint64
	Due      timing.VTimeInPicoSec
	Freq     timing.Freq
}

// Use wave State rather than engine-event details so this harness also runs on
// the previous release when measuring the effect of TickAt edge alignment.
func pendingSampledTasks(s *sim.Simulation) map[uint64]pendingSample {
	pending := map[uint64]pendingSample{}
	for _, c := range s.Components() {
		comp, ok := c.(*cu.Comp)
		if !ok {
			continue
		}
		predicted, _ := comp.Resources.Sampler.Predict()
		for _, w := range comp.State.Waves {
			if w.Execution.State == wavefront.WfSampledWaiting {
				pending[comp.State.WFTasks[w.ID]] = pendingSample{
					comp.Name(), w.ID, comp.State.WFTimes[w.ID] + predicted, comp.Spec.Freq,
				}
			}
		}
	}
	return pending
}

type sampledRun struct {
	tasks.NopObserver
	Events    []sampledTaskTiming
	Pending   map[uint64]pendingSample
	Completed map[uint64]timing.VTimeInPicoSec
}
type sampledTaskTiming struct {
	Location string                `json:"location"`
	Kind     string                `json:"kind"`
	Start    timing.VTimeInPicoSec `json:"start"`
	End      timing.VTimeInPicoSec `json:"end"`
}

func (r *sampledRun) TaskEnded(task tasks.Task) {
	if task.Kind == "wavefront" || (task.Kind == tasks.KindReqIn && strings.Contains(task.What, "LaunchKernel")) {
		r.Events = append(r.Events, sampledTaskTiming{
			task.Location, task.Kind + ":" + task.What, task.StartTime, task.EndTime,
		})
	}
	if _, ok := r.Pending[task.ID]; ok {
		r.Completed[task.ID] = task.EndTime
	}
}
func finishSampledFIR(t *testing.T, s *sim.Simulation, path string) {
	t.Helper()
	recorder := &sampledRun{Pending: pendingSampledTasks(s), Completed: map[uint64]timing.VTimeInPicoSec{}}
	if len(recorder.Pending) == 0 {
		t.Fatal("checkpoint has no pending sampled waves")
	}
	s.TaskTracker().Subscribe(recorder)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for task, pending := range recorder.Pending {
		done, ok := recorder.Completed[task]
		if !ok || done != pending.Freq.ThisTick(pending.Due) {
			t.Fatalf("wave %d: due=%d done=%d found=%v", pending.Wave, pending.Due, done, ok)
		}
		t.Logf("sampled %s wave=%d due=%d completed=%d alignment=%d ps",
			pending.Location, pending.Wave, pending.Due, done, done-pending.Due)
	}
	for _, event := range recorder.Events {
		t.Logf("%s %s start=%d end=%d duration=%d ps",
			event.Location, event.Kind, event.Start, event.End, event.End-event.Start)
	}
	data, err := json.Marshal(recorder.Events)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(path + ".tar.gz"); err != nil {
		t.Fatal(err)
	}
}
