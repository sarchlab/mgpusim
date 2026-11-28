package cu

import (
	"maps"
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

// taskLog records the tasks started and ended on a component.
type taskLog struct {
	started map[string]int // tasks started, by kind
	kinds   map[uint64]string
	insts   int // instructions ended
	waves   int // wavefronts ended
	// firstWFStart and lastWFEnd are the first wavefront's start and the last
	// one's end.
	firstWFStart, lastWFEnd timing.VTimeInPicoSec
	lastReqInEnd            timing.VTimeInPicoSec
}

func newTaskLog() *taskLog {
	return &taskLog{started: map[string]int{}, kinds: map[uint64]string{}}
}

func (l *taskLog) TaskStarted(task tasks.Task) {
	if task.Kind == "wavefront" && (l.started["wavefront"] == 0 || task.StartTime < l.firstWFStart) {
		l.firstWFStart = task.StartTime
	}
	l.started[task.Kind]++
	l.kinds[task.ID] = task.Kind
}

func (l *taskLog) TaskEnded(task tasks.Task) {
	switch l.kinds[task.ID] {
	case "inst":
		l.insts++
	case "wavefront":
		l.waves++
		l.lastWFEnd = max(l.lastWFEnd, task.EndTime)
	case tasks.KindReqIn:
		l.lastReqInEnd = max(l.lastReqInEnd, task.EndTime)
	}
}

func (l *taskLog) TagAdded(tasks.Tag)             {}
func (l *taskLog) MilestoneAdded(tasks.Milestone) {}

// The CPI stack, as --report-all collects it, covers the time from the first
// wavefront's start to the last one's end. Lines read ahead into the
// instruction cache stall no wavefront, so the tracer neither fails on them
// nor counts them, however late their replies arrive, even after the
// wavefronts have ended; nor does the stack grow as time goes on after them.
func TestCPIStackIgnoresInstructionPrefetches(t *testing.T) {
	const numWaves = 4
	code := straightLineCode(512)
	trace := func(prefetchLatency uint64) map[string]float64 {
		h := newFetchHarnessIn(t, true, mi300xFetchSpec(), code, numWaves)
		h.prefetchLatency = prefetchLatency
		tracer := NewCPIStackInstHook(h.cu.comp, h.engine)
		log := newTaskLog()
		h.sim.TaskTracker().Subscribe(tracer, h.cu.comp)
		h.sim.TaskTracker().Subscribe(log, h.cu.comp)
		// Track the work-group and its wavefronts as dispatch does: the
		// work-group's request ends after its last wavefront.
		req := h.wg.state().Request
		h.cu.comp.State.WGTasks[req.ID] = h.cu.comp.TaskTracker().StartReqIn(h.cu.comp, req)
		for _, w := range h.waves {
			h.cu.comp.State.WFTasks[w.UID] = h.cu.comp.TaskTracker().StartTask(h.cu.comp, tasks.StartInfo{
				Kind: "wavefront", What: "wavefront", Location: h.cu.comp.Name() + ".WFPool",
			})
		}
		// Hold the work-group's completion back for a while after its last
		// wavefront ends.
		h.ace.full = true
		for cycle := 0; log.waves < numWaves; cycle++ {
			if cycle == 4000 {
				t.Fatal("the wavefronts did not finish")
			}
			h.step()
		}
		atLastWave := tracer.GetCPIStack()
		for range 100 {
			h.step()
		}
		h.ace.full = false
		h.retiredAt(100)
		// Let late prefetch replies arrive after the wavefronts have ended.
		for range 3500 {
			h.step()
		}
		stack := tracer.GetCPIStack()

		if log.started["prefetch"] == 0 || log.started["fetch"] == 0 || log.waves != numWaves ||
			log.lastReqInEnd <= log.lastWFEnd {
			t.Fatalf("want fetches, prefetches, %d wavefronts and a later work-group end, got %v, "+
				"%d ended at %d and the work-group at %d", numWaves, log.started, log.waves, log.lastWFEnd,
				log.lastReqInEnd)
		}
		if !maps.Equal(stack, atLastWave) {
			t.Fatalf("the CPI stack grew from %v to %v after the wavefronts ended", atLastWave, stack)
		}
		freq := float64(h.cu.comp.Spec.Freq)
		want := secondsOf(log.lastWFEnd-log.firstWFStart) * freq / float64(log.insts)
		if stack["total"] != want {
			t.Fatalf("total CPI %.17g, want %.17g", stack["total"], want)
		}

		return stack
	}

	fast, slow := trace(0), trace(3000)
	if !maps.Equal(fast, slow) {
		t.Fatalf("with slow prefetches the CPI stack is %v, want %v", slow, fast)
	}
}
