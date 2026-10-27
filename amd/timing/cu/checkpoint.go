package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func wfID(w *Wavefront) uint64 {
	if w == nil {
		return 0
	}
	return w.handle
}
func instID(i *wavefront.Inst) uint64 {
	if i == nil {
		return 0
	}
	return i.ID
}

func (cu *ComputeUnit) wave(id uint64) *Wavefront {
	if id == 0 {
		return nil
	}
	index := slotIndex(id)
	if index < 0 || index >= len(cu.comp.State.Waves) {
		panic("cu: invalid wave slot")
	}
	state := &cu.comp.State.Waves[index]
	if state.ID == 0 || state.Generation != uint32(id>>32) {
		panic("cu: stale wave slot")
	}
	return &Wavefront{cu: cu, UID: state.ID, handle: id}
}

func (cu *ComputeUnit) instruction(id uint64) *wavefront.Inst {
	if id == 0 {
		return nil
	}
	v, ok := cu.comp.State.Instructions[id]
	if !ok {
		panic(fmt.Sprintf("cu: missing instruction %d", id))
	}
	raw, err := cu.decodeDescription(v.Raw)
	if err != nil {
		panic(fmt.Sprintf("cu: decode instruction %d: %v", id, err))
	}
	// One operation-local allocation holds both the immutable description copy
	// and its dynamic identity; neither escapes into middleware state.
	if _, ok := cu.Decoder.(*insts.Disassembler); ok {
		value := &struct {
			dynamic wavefront.Inst
			decoded insts.Inst
		}{decoded: *raw}
		value.decoded.PC = v.PC
		value.dynamic = wavefront.Inst{Inst: &value.decoded, ID: id, Raw: v.Raw}
		return &value.dynamic
	}
	raw.PC = v.PC
	i := wavefront.NewInst(id, raw)
	i.Raw = v.Raw
	return i
}

func (cu *ComputeUnit) rememberInstruction(owner uint64, i *wavefront.Inst) {
	if i == nil {
		return
	}
	if cu.comp.State.Instructions == nil {
		cu.comp.State.Instructions = make(map[uint64]InstructionState)
	}
	// Decoded fields are immutable views of this canonical encoding.
	var pc uint64
	if i.Inst != nil {
		pc = i.PC
	}
	value := cu.comp.State.Instructions[i.ID]
	if value.Owner != 0 && value.Owner != owner {
		panic("cu: instruction belongs to another wave slot")
	}
	value.ID, value.PC, value.Raw, value.Owner = i.ID, pc, i.Raw, owner
	cu.comp.State.Instructions[i.ID] = value
}

func (cu *ComputeUnit) allocateGroup(raw kernels.WorkGroupValue, req messaging.Msg) *WorkGroup {
	for index := range cu.comp.State.Groups {
		if cu.comp.State.Groups[index].Occupied {
			continue
		}
		cu.comp.State.Groups[index] = WorkGroupState{Occupied: true, Raw: raw, Request: req}
		return &WorkGroup{cu: cu, UID: raw.UID, slot: index}
	}
	panic("cu: hardware work-group slots exhausted")
}
func (cu *ComputeUnit) allocateWave(group *WorkGroup, index int, value WaveState) *Wavefront {
	for slot := range cu.comp.State.Waves {
		old := &cu.comp.State.Waves[slot]
		if old.ID != 0 {
			continue
		}
		generation := old.Generation + 1
		if generation == 0 {
			panic("cu: wave slot generation exhausted")
		}
		value.Generation, value.Group, value.Index = generation, group.slot, index
		*old = value
		handle := waveHandle(slot, generation)
		group.state().Waves = append(group.state().Waves, handle)
		return &Wavefront{cu: cu, UID: value.ID, handle: handle}
	}
	panic("cu: hardware wavefront slots exhausted")
}
func (cu *ComputeUnit) newGroup(raw *kernels.WorkGroup, req messaging.Msg) *WorkGroup {
	free := 0
	for i := range cu.comp.State.Waves {
		if cu.comp.State.Waves[i].ID == 0 {
			free++
		}
	}
	if free < len(raw.Wavefronts) {
		panic("cu: hardware wavefront slots exhausted")
	}
	group := cu.allocateGroup(kernels.WorkGroupSnapshot(raw), req)
	group.state().LDS = make([]byte, raw.Packet.GroupSegmentSize)
	for i, w := range raw.Wavefronts {
		value := WaveState{ID: w.UID, HasScoreboard: cu.comp.Spec.RegisterScoreboard}
		value.Execution.SampleIssueTime = w.IssueTime
		value.Execution.SampleFinishTime = w.FinishTime
		value.Execution.PID = req.Payload.(protocol.MapWGReq).PID
		value.Execution.InstBuffer = make([]byte, 0, 256)
		cu.allocateWave(group, i, value)
	}
	return group
}

// useWave is the stable reference stored by pipeline and queue state.
func (cu *ComputeUnit) useWave(w *Wavefront) uint64 {
	if w == nil {
		return 0
	}
	if w.cu != cu {
		panic("cu: wavefront is not owned by this CU")
	}
	return w.handle
}

func (cu *ComputeUnit) fetchState(v *InstFetchReqInfo) FetchState {
	return FetchState{v.Req, cu.useWave(v.Wavefront), v.Address, v.FetchTaskID}
}
func (cu *ComputeUnit) scalarState(v *ScalarMemAccessInfo) ScalarAccessState {
	cu.rememberInstruction(cu.useWave(v.Wavefront), v.Inst)
	return ScalarAccessState{v.Req, cu.useWave(v.Wavefront), instID(v.Inst), v.DstSGPR.RegType}
}
func (cu *ComputeUnit) vectorState(v VectorMemAccessInfo) VectorAccessState {
	cu.rememberInstruction(cu.useWave(v.Wavefront), v.Inst)
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

// releaseInstruction checks the instruction's owning slot and execution state.
// No tracing status or CU-wide holder search participates in instruction lifetime.
func (cu *ComputeUnit) releaseInstruction(id uint64) {
	value, exists := cu.comp.State.Instructions[id]
	if !exists || value.ExecutionPending {
		return
	}
	if value.Owner != 0 {
		owner := &cu.comp.State.Waves[slotIndex(value.Owner)]
		if owner.ID != 0 && owner.Generation == uint32(value.Owner>>32) &&
			(owner.Execution.InstID == id || owner.Execution.ToIssueID == id) {
			return
		}
	}
	delete(cu.comp.State.Instructions, id)
}

// executionEnded differs from fully drained: prefetch can outlive S_ENDPGM.
func executionEnded(state wavefront.WfState) bool {
	return state == wavefront.WfDraining || state == wavefront.WfCompleted
}

// Completion retains the original hardware slots until all dependent work
// drains. This deliberately conservative timing model also delays CP release.
func (cu *ComputeUnit) retireGroups() {
	for index := range cu.comp.State.Groups {
		group := &cu.comp.State.Groups[index]
		if !group.Occupied || len(group.Waves) == 0 {
			continue
		}
		drained := true
		for _, handle := range group.Waves {
			if cu.wave(handle).execution().State != wavefront.WfCompleted {
				drained = false
				break
			}
		}
		if !drained {
			continue
		}
		wg := &WorkGroup{cu: cu, UID: group.Raw.UID, slot: index}
		if !cu.sendWGCompletionMessage(wg) {
			cu.comp.TickLater()
			continue
		}
		for _, handle := range group.Waves {
			cu.endWaveTask(cu.wave(handle))
		}
		cu.clearWGResource(wg)
		cu.completeWGTask(group.Request.ID)
		cu.retireGroup(index)
	}
}
func (cu *ComputeUnit) retireGroup(index int) {
	s := &cu.comp.State
	group := &s.Groups[index]
	for _, handle := range group.Waves {
		wave := &s.Waves[slotIndex(handle)]
		inst, next := wave.Execution.InstID, wave.Execution.ToIssueID
		delete(s.WFTimes, wave.ID)
		*wave = WaveState{Generation: wave.Generation}
		cu.releaseInstruction(inst)
		cu.releaseInstruction(next)
	}
	*group = WorkGroupState{}
}
