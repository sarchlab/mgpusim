package cu

import (
	"fmt"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// This independent test sweep deliberately examines physical holders rather
// than trusting the production FSM. It runs only at stable operation boundaries.
func assertLifecycleHolders(t *testing.T, c *ComputeUnit) {
	t.Helper()
	holders := testOperationHolders(c)
	for _, evt := range testPendingCompletions(t, c) {
		holders[evt.Wave] = append(holders[evt.Wave], "sample event")
	}
	for handle, locations := range holders {
		w := c.wave(handle) // validates occupied slot and generation
		if w.execution().State == wavefront.WfCompleted {
			t.Fatalf("completed wave %d still held by %v", handle, locations)
		}
	}
	for _, pool := range c.comp.State.Pools {
		for _, handle := range pool {
			c.wave(handle)
		}
	}
	assertWaveDrainState(t, c)
	assertInstructionReferences(t, c)
	for id, instruction := range c.comp.State.Instructions {
		w := c.wave(instruction.Owner)
		if w == nil {
			t.Fatalf("instruction %d has no owner", id)
		}
		if !instruction.ExecutionPending && w.execution().InstID != id && w.execution().ToIssueID != id {
			t.Fatalf("completed instruction %d leaked outside its owning slot", id)
		}
		if instruction.ExecutionPending && executionEnded(w.execution().State) {
			t.Fatalf("execution-ended wave retains pending instruction %d", id)
		}
	}
}

func assertWaveDrainState(t *testing.T, c *ComputeUnit) {
	t.Helper()
	for i := range c.comp.State.Waves {
		w := &c.comp.State.Waves[i]
		if w.ID == 0 {
			continue
		}
		state := &w.Execution
		handle := makeWaveHandle(i, w.Generation)
		fetches := 0
		for _, f := range c.comp.State.InstFetch {
			if f.WF == handle {
				fetches++
			}
		}
		if fetches > 1 || state.IsFetching != (fetches == 1) {
			t.Fatalf("fetch state disagrees with transaction queue for %d", handle)
		}
		if executionEnded(state.State) && (!w.StopFetch ||
			state.OutstandingScalarMemAccess != 0 || state.OutstandingVectorMemAccess != 0) {
			t.Fatalf("execution-ended wave %d has unfinished data operations", handle)
		}
		if state.State == wavefront.WfDraining && !state.IsFetching {
			t.Fatalf("draining wave %d has no pending fetch", handle)
		}
	}
}

func testOperationHolders(c *ComputeUnit) map[waveHandle][]string {
	s := &c.comp.State
	holders := make(map[waveHandle][]string)
	add := func(id waveHandle, where string) {
		if id != 0 {
			holders[id] = append(holders[id], where)
		}
	}
	for _, d := range s.Decoders {
		for _, id := range d.ToDecode {
			add(id, "decoder")
		}
	}
	for _, stage := range []StageState{s.Branch, s.LDS, s.ScalarUnit.Stages} {
		add(stage.Read, "read stage")
		add(stage.Exec, "exec stage")
		add(stage.Write, "write stage")
	}
	for _, simd := range s.SIMDs {
		add(simd.Exec, "SIMD")
		for _, slot := range simd.Slots {
			add(slot.WF, "SIMD slot")
		}
	}
	for _, id := range s.Scheduler.Internal {
		add(id, "internal")
	}
	for _, id := range s.Scheduler.Barrier {
		add(id, "barrier")
	}
	testMemoryHolders(s, add)
	return holders
}

func testMemoryHolders(s *State, add func(waveHandle, string)) {
	for _, f := range s.InstFetch {
		add(f.WF, "fetch")
	}
	for _, a := range s.Scalar {
		add(a.WF, "scalar memory")
	}
	requests := make(map[uint64]waveHandle)
	for _, a := range s.Vector {
		add(a.WF, "vector memory")
		requests[a.requestID()] = a.WF
	}
	for _, a := range s.VectorUnit.Waiting {
		add(a.WF, "coalescing")
	}
	for _, stage := range s.VectorUnit.InstructionsPipeline.Stages() {
		add(stage.Item.Handle, "vector instruction pipeline")
	}
	for i := 0; i < s.VectorUnit.InstructionsOutput.Size(); i++ {
		v, _ := s.VectorUnit.InstructionsOutput.PeekAt(i)
		add(v.Handle, "vector instruction output")
	}
	txn := func(id uint64, where string) {
		if id == 0 {
			return
		}
		handle, ok := requests[id]
		if !ok {
			panic(fmt.Sprintf("%s has orphan transaction %d", where, id))
		}
		add(handle, where)
	}
	for _, stage := range s.VectorUnit.TransactionsPipeline.Stages() {
		txn(stage.Item, "transaction pipeline")
	}
	for i := 0; i < s.VectorUnit.TransactionsOutput.Size(); i++ {
		id, _ := s.VectorUnit.TransactionsOutput.PeekAt(i)
		txn(id, "transaction output")
	}
	for _, req := range s.ScalarUnit.Reads {
		found := false
		for _, a := range s.Scalar {
			if a.Req.ID == req.ID {
				add(a.WF, "scalar request buffer")
				found = true
				break
			}
		}
		if !found {
			panic("scalar request buffer has orphan transaction")
		}
	}
}

func testPendingCompletions(t *testing.T, c *ComputeUnit) []SampledCompletion {
	t.Helper()
	return c.comp.State.SampledCompletions
}

// Check from the holders toward the encoding map too: a missing encoding would
// otherwise escape an audit that only iterates surviving Instructions entries.
func assertInstructionReferences(t *testing.T, c *ComputeUnit) {
	t.Helper()
	check := func(id uint64, owner waveHandle, pending bool) {
		instruction, exists := c.comp.State.Instructions[id]
		if !exists || instruction.Owner != owner || (pending && !instruction.ExecutionPending) {
			t.Fatalf("instruction %d lost owner %d or pending execution", id, owner)
		}
	}
	for i := range c.comp.State.Waves {
		w := &c.comp.State.Waves[i]
		if w.ID == 0 {
			continue
		}
		for _, id := range []uint64{w.Execution.InstID, w.Execution.ToIssueID} {
			if id != 0 {
				check(id, makeWaveHandle(i, w.Generation), false)
			}
		}
	}
	for _, a := range c.comp.State.Scalar {
		check(a.Inst, a.WF, true)
	}
	for _, a := range c.comp.State.Vector {
		check(a.Inst, a.WF, true)
	}
	for _, a := range c.comp.State.VectorUnit.Waiting {
		check(a.Inst, a.WF, true)
	}
}
