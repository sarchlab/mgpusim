package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestCPIStackIndependentOfLowerPriorityEventSplits(t *testing.T) {
	trace := func(split bool) map[string]float64 {
		component := new(Comp)
		component.Spec.Freq = timing.GHz
		hook := NewCPIStackInstHook(component, nil)
		hook.TaskStarted(tasks.Task{ID: 1, Kind: "wavefront"})
		var end timing.VTimeInPicoSec
		for i := uint64(0); i < 100; i++ {
			start := timing.VTimeInPicoSec(1_000_000 + i*2_000_009)
			end = start + 1_000_003
			branch := tasks.Task{ID: 2, Kind: "inst", What: "Branch", StartTime: start}
			hook.TaskStarted(branch)
			if split {
				// Fetch activity cannot change the higher-priority branch's
				// attributed time or the number of retired instructions.
				for offset := timing.VTimeInPicoSec(7); offset < 1_000_003; offset += 7919 {
					fetch := tasks.Task{ID: 3, Kind: "fetch", What: "fetch", StartTime: start + offset}
					hook.TaskStarted(fetch)
					fetch.EndTime = fetch.StartTime + 1
					hook.TaskEnded(fetch)
				}
			}
			branch.EndTime = end
			hook.TaskEnded(branch)
		}
		hook.TaskEnded(tasks.Task{ID: 1, Kind: "wavefront", EndTime: end})
		return hook.GetCPIStack()
	}

	unsplit, split := trace(false), trace(true)
	for _, key := range []string{"Branch", "total"} {
		if unsplit[key] != split[key] {
			t.Fatalf("%s changed with lower-priority event splits: %.17g != %.17g",
				key, unsplit[key], split[key])
		}
	}
}

// kindCounter counts the tasks of each kind started on a component.
type kindCounter map[string]int

func (k kindCounter) TaskStarted(task tasks.Task)    { k[task.Kind]++ }
func (k kindCounter) TaskEnded(tasks.Task)           {}
func (k kindCounter) TagAdded(tasks.Tag)             {}
func (k kindCounter) MilestoneAdded(tasks.Milestone) {}

// The CPI stack, as --report-all collects it, accounts for wavefront time
// only: lines read ahead into the instruction cache stall no wavefront, so
// the tracer neither fails on them nor counts them, however late their
// replies arrive, even after the wavefront has ended.
func TestCPIStackIgnoresInstructionPrefetches(t *testing.T) {
	code := straightLineCode(512)
	trace := func(prefetchLatency uint64) (map[string]float64, kindCounter) {
		h := newFetchHarnessIn(t, true, mi300xFetchSpec(), code, 1)
		h.prefetchLatency = prefetchLatency
		tracer := NewCPIStackInstHook(h.cu.comp, h.engine)
		kinds := kindCounter{}
		h.sim.TaskTracker().Subscribe(tracer, h.cu.comp)
		h.sim.TaskTracker().Subscribe(kinds, h.cu.comp)
		for _, w := range h.waves {
			h.cu.comp.State.WFTasks[w.UID] = h.cu.comp.TaskTracker().StartTask(h.cu.comp, tasks.StartInfo{
				Kind: "wavefront", What: "wavefront", Location: h.cu.comp.Name() + ".WFPool",
			})
		}
		h.retiredAt(2000)
		// Let late prefetch replies arrive after the wavefront has ended.
		for range prefetchLatency {
			h.step()
		}

		return tracer.GetCPIStack(), kinds
	}

	fast, kinds := trace(0)
	if kinds["prefetch"] == 0 || kinds["fetch"] == 0 || kinds["inst"] == 0 {
		t.Fatalf("want fetch, prefetch and instruction tasks, got %v", kinds)
	}
	slow, _ := trace(3000)
	if len(fast) != len(slow) {
		t.Fatalf("with slow prefetches the CPI stack is %v, want %v", slow, fast)
	}
	for key, value := range fast {
		if slow[key] != value {
			t.Fatalf("with slow prefetches the CPI stack is %v, want %v", slow, fast)
		}
	}
}
