package cu

import (
	"maps"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
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
		trackGroup(h, h.wg)
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

// trackGroup starts the tasks of the work-group and its wavefronts, as
// dispatch does: the work-group's request ends after its last wavefront.
func trackGroup(h *fetchHarness, group *WorkGroup) {
	req := group.state().Request
	h.cu.comp.State.WGTasks[req.ID] = h.cu.comp.TaskTracker().StartReqIn(h.cu.comp, req)
	for _, w := range group.waves() {
		h.cu.comp.State.WFTasks[w.UID] = h.cu.comp.TaskTracker().StartTask(h.cu.comp, tasks.StartInfo{
			Kind: "wavefront", What: "wavefront", Location: h.cu.comp.Name() + ".WFPool",
		})
	}
}

// checkStackAddsUp checks that the components of the CPI stack add up to the
// time from the first wavefront's start to the last one's end, and that the
// total is that time per instruction.
func checkStackAddsUp(t *testing.T, tracer *CPIStackTracer, log *taskLog) {
	t.Helper()
	var charged timing.VTimeInPicoSec
	for _, d := range tracer.timeStack {
		charged += d
	}
	span := log.lastWFEnd - log.firstWFStart
	if charged != span {
		t.Fatalf("the components add up to %d ps, want %d ps from the first wavefront's start to the "+
			"last one's end", charged, span)
	}
	want := secondsOf(span) * float64(tracer.cu.Spec.Freq) / float64(log.insts)
	if total := tracer.GetCPIStack()["total"]; total != want {
		t.Fatalf("total CPI %.17g, want %.17g", total, want)
	}
}

// lateFetchRun runs work-groups of wavefronts whose code object runs on past
// their s_endpgm, so they fetch ahead into it, and holds back the replies to
// the fetches past the s_endpgm while hold is set.
type lateFetchRun struct {
	t       *testing.T
	h       *fetchHarness
	tracer  *CPIStackTracer
	log     *taskLog
	codeEnd uint64
}

func newLateFetchRun(t *testing.T, numWaves int) *lateFetchRun {
	t.Helper()
	program := straightLineCode(256)
	code := append(program, make([]byte, 256)...)
	h := newFetchHarnessIn(t, true, mi300xFetchSpec(), code, numWaves)
	r := &lateFetchRun{t: t, h: h, tracer: NewCPIStackInstHook(h.cu.comp, h.engine), log: newTaskLog(),
		codeEnd: h.base + uint64(len(program))}
	h.sim.TaskTracker().Subscribe(r.tracer, h.cu.comp)
	h.sim.TaskTracker().Subscribe(r.log, h.cu.comp)
	trackGroup(h, h.wg)
	r.hold(true)

	return r
}

func (r *lateFetchRun) hold(on bool) {
	r.h.hold = nil
	if on {
		r.h.hold = func(read memprotocol.ReadReq) bool {
			return read.AccessByteSize != r.h.cu.instLineByteSize() && read.Address >= r.codeEnd
		}
	}
}

func (r *lateFetchRun) steps(n int) {
	for range n {
		r.h.step()
	}
}

func (r *lateFetchRun) stepUntil(done func() bool) {
	r.t.Helper()
	for cycle := 0; !done(); cycle++ {
		if cycle == 4000 {
			r.t.Fatal("the work did not finish")
		}
		r.h.step()
	}
}

// checkFetchHeld checks that a fetch past the s_endpgm is in flight.
func (r *lateFetchRun) checkFetchHeld() {
	r.t.Helper()
	for _, f := range r.h.cu.comp.State.InstFetch {
		if !f.Prefetch && f.Address >= r.codeEnd {
			return
		}
	}
	r.t.Fatal("no fetch past the s_endpgm is in flight")
}

// A wavefront can end with a fetch for the bytes past its s_endpgm still in
// flight. The CPI stack stops at the last wavefront's end, so however late
// that reply arrives, it charges no fetch time; and when another work-group
// starts after a gap, the gap is inside the total, which its components add up
// to.
func TestCPIStackStopsAtTheLastWavefrontsEnd(t *testing.T) {
	const numWaves = 2
	r := newLateFetchRun(t, numWaves)
	r.stepUntil(func() bool { return r.log.waves == numWaves })
	r.checkFetchHeld()
	atEnd := r.tracer.GetCPIStack()
	r.steps(300)
	r.hold(false)
	r.stepUntil(func() bool { return len(r.h.ace.sent) == 1 })
	r.steps(300)
	if stack := r.tracer.GetCPIStack(); !maps.Equal(stack, atEnd) {
		t.Fatalf("a late fetch reply changed the CPI stack from %v to %v", atEnd, stack)
	}
	checkStackAddsUp(t, r.tracer, r.log)

	trackGroup(r.h, r.h.addGroup(numWaves))
	r.stepUntil(func() bool { return len(r.h.ace.sent) == 2 })
	if r.log.waves != 2*numWaves {
		t.Fatalf("%d wavefronts ended, want %d", r.log.waves, 2*numWaves)
	}
	checkStackAddsUp(t, r.tracer, r.log)
}

// While no wavefront runs, the compute unit is idle, even with a fetch for an
// ended wavefront still in flight: when the next work-group starts before the
// reply arrives, the gap is charged to Idle, not to Fetch.
func TestCPIStackChargesTheGapBetweenWorkGroupsToIdle(t *testing.T) {
	const numWaves = 2
	r := newLateFetchRun(t, numWaves)
	r.stepUntil(func() bool { return r.log.waves == numWaves })
	lastEnd := r.log.lastWFEnd
	r.steps(100)
	r.checkFetchHeld()
	idle, fetch := r.tracer.timeStack["Idle"], r.tracer.timeStack["Fetch"]

	trackGroup(r.h, r.h.addGroup(numWaves))
	gap := r.h.engine.CurrentTime() - lastEnd
	if got := r.tracer.timeStack["Idle"] - idle; got != gap {
		t.Fatalf("the next work-group's start charged %d ps to Idle, want the %d ps gap", got, gap)
	}
	if got := r.tracer.timeStack["Fetch"]; got != fetch {
		t.Fatalf("the next work-group's start moved Fetch from %d ps to %d ps", fetch, got)
	}
	r.steps(1)
	if got := r.tracer.timeStack["Fetch"] - fetch; got >= gap {
		t.Fatalf("the cycle after the next work-group's start charged %d ps to Fetch", got)
	}

	r.steps(50)
	r.hold(false)
	r.stepUntil(func() bool { return r.log.waves == 2*numWaves })
	atEnd := r.tracer.GetCPIStack()
	r.stepUntil(func() bool { return len(r.h.ace.sent) == 2 })
	r.steps(300)
	if stack := r.tracer.GetCPIStack(); !maps.Equal(stack, atEnd) {
		t.Fatalf("the CPI stack changed from %v to %v after the last wavefront ended", atEnd, stack)
	}
	checkStackAddsUp(t, r.tracer, r.log)
}
