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
