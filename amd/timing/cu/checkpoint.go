package cu

import (
	"fmt"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
	"slices"
)

func wfID(w *wavefront.Wavefront) uint64 {
	if w == nil {
		return 0
	}
	return w.UID
}
func instID(i *wavefront.Inst) uint64 {
	if i == nil {
		return 0
	}
	return i.ID
}

// ensureState invalidates only derived views after component checkpoint loading.
// ReadCheckpoint replaces State, including its decoder slice. No execution data
// is copied back from a view: every mutable value already belongs to State.
func (cu *ComputeUnit) ensureState() {
	if len(cu.comp.State.Decoders) == 0 {
		return
	}
	if cu.boundDecoder != &cu.comp.State.Decoders[0] {
		cu.bindState()
	}
}

func (cu *ComputeUnit) bindState() {
	s := &cu.comp.State
	if len(s.Decoders) != 4 || len(s.Pools) != numWfPools ||
		len(s.SIMDs) != cu.comp.Spec.SIMDCount || len(s.VRegisters) != cu.comp.Spec.SIMDCount {
		panic("cu: invalid state geometry")
	}
	cu.boundDecoder = &s.Decoders[0]
	cu.workGroups = make(map[uint64]*wavefront.WorkGroup)
	cu.waves = make(map[uint64]*wavefront.Wavefront)
	cu.instructions = make(map[uint64]*wavefront.Inst)
	for id, g := range s.Groups {
		raw, err := g.Raw.Restore()
		if err != nil {
			panic(err)
		}
		cu.bindGroup(id, raw, g)
	}
}

func (cu *ComputeUnit) bindGroup(id uint64, raw *kernels.WorkGroup, g WorkGroupState) *wavefront.WorkGroup {
	wg := wavefront.NewWorkGroup(raw, g.Request)
	wg.LDS = g.LDS
	if len(raw.Wavefronts) != len(g.Waves) {
		panic("cu: wavefront count mismatch")
	}
	cu.workGroups[id] = wg
	for i, base := range raw.Wavefronts {
		value := &g.Waves[i]
		if value.ID != base.UID {
			panic("cu: wavefront identity mismatch")
		}
		wf := wavefront.NewWavefront(base)
		wf.BindExecution(&value.Execution, cu.instruction, cu.rememberInstruction, cu.releaseInstruction)
		wf.WG = wg
		wf.RegAccessor = &CURegFileAccessor{CU: cu, WF: wf}
		if value.HasScoreboard {
			wf.ScoreboardData = &value.Scoreboard
		}
		wg.Wfs = append(wg.Wfs, wf)
		cu.waves[value.ID] = wf
	}
	return wg
}

func (cu *ComputeUnit) wave(id uint64) *wavefront.Wavefront {
	if id == 0 {
		return nil
	}
	cu.ensureState()
	w := cu.waves[id]
	if w == nil {
		panic(fmt.Sprintf("cu: unknown wavefront %d", id))
	}
	return w
}

func (cu *ComputeUnit) instruction(id uint64) *wavefront.Inst {
	if id == 0 {
		return nil
	}
	cu.ensureState()
	if i := cu.instructions[id]; i != nil {
		return i
	}
	v, ok := cu.comp.State.Instructions[id]
	if !ok {
		panic(fmt.Sprintf("cu: missing instruction %d", id))
	}
	raw, err := cu.Decoder.Decode(v.Raw)
	if err != nil {
		panic(fmt.Sprintf("cu: decode instruction %d: %v", id, err))
	}
	raw.PC = v.PC
	i := wavefront.NewInst(id, raw)
	i.Raw = v.Raw
	cu.instructions[id] = i
	return i
}

func (cu *ComputeUnit) rememberInstruction(i *wavefront.Inst) {
	if i == nil {
		return
	}
	if cu.instructions == nil {
		cu.instructions = make(map[uint64]*wavefront.Inst)
	}
	if cu.comp.State.Instructions == nil {
		cu.comp.State.Instructions = make(map[uint64]InstructionState)
	}
	// Decoded fields are immutable views of this canonical encoding.
	var pc uint64
	if i.Inst != nil {
		pc = i.PC
	}
	cu.comp.State.Instructions[i.ID] = InstructionState{ID: i.ID, PC: pc, Raw: i.Raw}
	cu.instructions[i.ID] = i
}

func (cu *ComputeUnit) newGroup(raw *kernels.WorkGroup, req messaging.Msg) *wavefront.WorkGroup {
	cu.ensureState()
	g := WorkGroupState{
		Raw: kernels.WorkGroupSnapshot(raw), Request: req,
		LDS: make([]byte, raw.Packet.GroupSegmentSize), Waves: make([]WaveState, len(raw.Wavefronts)),
	}
	for i, w := range raw.Wavefronts {
		g.Waves[i].ID = w.UID
		g.Waves[i].Execution.SampleIssueTime = w.IssueTime
		g.Waves[i].Execution.SampleFinishTime = w.FinishTime
		g.Waves[i].Execution.PID = req.Payload.(protocol.MapWGReq).PID
		g.Waves[i].Execution.InstBuffer = make([]byte, 0, 256)
		g.Waves[i].HasScoreboard = cu.comp.Spec.RegisterScoreboard
	}
	cu.comp.State.Groups[raw.UID] = g
	return cu.bindGroup(raw.UID, raw, g)
}

// useWave is the stable reference stored by pipeline and queue state.
func (cu *ComputeUnit) useWave(w *wavefront.Wavefront) uint64 {
	if w == nil {
		return 0
	}
	if cu.waves[w.UID] != w {
		panic("cu: wavefront is not owned by this CU")
	}
	return w.UID
}

func (cu *ComputeUnit) fetchState(v *InstFetchReqInfo) FetchState {
	return FetchState{v.Req, cu.useWave(v.Wavefront), v.Address, v.FetchTaskID}
}
func (cu *ComputeUnit) scalarState(v *ScalarMemAccessInfo) ScalarAccessState {
	cu.rememberInstruction(v.Inst)
	return ScalarAccessState{v.Req, cu.useWave(v.Wavefront), instID(v.Inst), v.DstSGPR.RegType}
}
func (cu *ComputeUnit) vectorState(v VectorMemAccessInfo) VectorAccessState {
	cu.rememberInstruction(v.Inst)
	s := VectorAccessState{Read: v.Read, Write: v.Write, WF: cu.useWave(v.Wavefront), Inst: instID(v.Inst)}
	for _, l := range v.laneInfo {
		s.Lanes = append(s.Lanes, LaneState{l.laneID, l.reg.RegType, l.regCount, l.addrOffsetInCacheLine})
	}
	return s
}
func (v VectorAccessState) requestID() uint64 {
	if v.Read.ID != 0 {
		return v.Read.ID
	}
	return v.Write.ID
}

// releaseInstruction retires an encoding only after its execution and wave
// references disappear. In-flight memory operations retain their inst task.
func (cu *ComputeUnit) releaseInstruction(id uint64) {
	if id == 0 {
		return
	}
	if _, live := cu.comp.State.InstTasks[id]; live {
		return
	}
	for _, g := range cu.comp.State.Groups {
		for _, w := range g.Waves {
			if w.Execution.InstID == id || w.Execution.ToIssueID == id {
				return
			}
		}
	}
	for _, records := range [][]ScalarAccessState{cu.comp.State.Scalar, cu.comp.State.ShadowScalar} {
		for _, record := range records {
			if record.Inst == id {
				return
			}
		}
	}
	for _, records := range [][]VectorAccessState{
		cu.comp.State.Vector, cu.comp.State.ShadowVector, cu.comp.State.VectorUnit.Waiting,
	} {
		for _, record := range records {
			if record.Inst == id {
				return
			}
		}
	}
	delete(cu.comp.State.Instructions, id)
	delete(cu.instructions, id)
}

// Completion releases scheduling resources immediately, but fetch-ahead replies
// may still refer to a completed wave. Retire its canonical values only after
// the last live queue/stage reference has disappeared.
func (cu *ComputeUnit) retireGroups() {
	for id, g := range cu.comp.State.Groups {
		if !g.Completed {
			continue
		}
		live := false
		for _, w := range g.Waves {
			if cu.waveReferenced(w.ID) {
				live = true
				break
			}
		}
		if live {
			continue
		}
		delete(cu.comp.State.Groups, id)
		delete(cu.workGroups, id)
		for _, w := range g.Waves {
			delete(cu.waves, w.ID)
			delete(cu.comp.State.WFTimes, w.ID)
			cu.releaseInstruction(w.Execution.InstID)
			cu.releaseInstruction(w.Execution.ToIssueID)
		}
	}
}
func (cu *ComputeUnit) waveReferenced(id uint64) bool {
	s := &cu.comp.State
	for _, ids := range s.Pools {
		if slices.Contains(ids, id) {
			return true
		}
	}
	if slices.Contains(s.Scheduler.Internal, id) || slices.Contains(s.Scheduler.Barrier, id) {
		return true
	}
	for _, d := range s.Decoders {
		for _, w := range d.ToDecode {
			if w == id {
				return true
			}
		}
	}
	for _, stage := range []StageState{s.Branch, s.LDS, s.ScalarUnit.Stages} {
		if stage.Read == id || stage.Exec == id || stage.Write == id {
			return true
		}
	}
	for _, simd := range s.SIMDs {
		if simd.Exec == id {
			return true
		}
		for _, slot := range simd.Slots {
			if slot.WF == id {
				return true
			}
		}
	}
	if cu.waveInMemory(id) {
		return true
	}
	for _, stage := range s.VectorUnit.InstructionsPipeline.Stages() {
		if stage.Item.WavefrontID == id {
			return true
		}
	}
	for i := 0; i < s.VectorUnit.InstructionsOutput.Size(); i++ {
		v, _ := s.VectorUnit.InstructionsOutput.PeekAt(i)
		if v.WavefrontID == id {
			return true
		}
	}
	return false
}

func (cu *ComputeUnit) waveInMemory(id uint64) bool {
	s := &cu.comp.State
	for _, xs := range [][]FetchState{s.InstFetch, s.ShadowInstFetch} {
		for _, x := range xs {
			if x.WF == id {
				return true
			}
		}
	}
	for _, xs := range [][]ScalarAccessState{s.Scalar, s.ShadowScalar} {
		for _, x := range xs {
			if x.WF == id {
				return true
			}
		}
	}
	for _, xs := range [][]VectorAccessState{s.Vector, s.ShadowVector, s.VectorUnit.Waiting} {
		for _, x := range xs {
			if x.WF == id {
				return true
			}
		}
	}
	return false
}
