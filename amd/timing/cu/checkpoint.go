package cu

import (
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func wfID(w *Wavefront) uint64 {
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

func (cu *ComputeUnit) wave(id uint64) *Wavefront {
	if id == 0 {
		return nil
	}
	w := cu.findWavefront(id)
	if w == nil {
		panic(fmt.Sprintf("cu: unknown wavefront %d", id))
	}
	return w
}

func (cu *ComputeUnit) instruction(id uint64) *wavefront.Inst {
	if id == 0 {
		return nil
	}
	v, ok := cu.comp.State.Instructions[id]
	if !ok {
		panic(fmt.Sprintf("cu: missing instruction %d", id))
	}
	raw, err := cu.decodeInstruction(v.Raw)
	if err != nil {
		panic(fmt.Sprintf("cu: decode instruction %d: %v", id, err))
	}
	raw.PC = v.PC
	i := wavefront.NewInst(id, raw)
	i.Raw = v.Raw
	return i
}

func (cu *ComputeUnit) rememberInstruction(i *wavefront.Inst) {
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
	cu.comp.State.Instructions[i.ID] = InstructionState{ID: i.ID, PC: pc, Raw: i.Raw}
}

func (cu *ComputeUnit) newGroup(raw *kernels.WorkGroup, req messaging.Msg) *WorkGroup {
	g := WorkGroupState{
		Raw: kernels.WorkGroupSnapshot(raw), Request: req,
		LDS: make([]byte, raw.Packet.GroupSegmentSize), Waves: make([]WaveState, len(raw.Wavefronts)),
	}
	for i, w := range raw.Wavefronts {
		cu.comp.State.WaveLocations[w.UID] = WaveLocation{Group: len(cu.comp.State.Groups), Index: i}
		g.Waves[i].ID = w.UID
		g.Waves[i].Execution.SampleIssueTime = w.IssueTime
		g.Waves[i].Execution.SampleFinishTime = w.FinishTime
		g.Waves[i].Execution.PID = req.Payload.(protocol.MapWGReq).PID
		g.Waves[i].Execution.InstBuffer = make([]byte, 0, 256)
		g.Waves[i].HasScoreboard = cu.comp.Spec.RegisterScoreboard
	}
	cu.comp.State.GroupLocations[raw.UID] = len(cu.comp.State.Groups)
	cu.comp.State.Groups = append(cu.comp.State.Groups, g)
	return &WorkGroup{cu: cu, UID: raw.UID}
}

// useWave is the stable reference stored by pipeline and queue state.
func (cu *ComputeUnit) useWave(w *Wavefront) uint64 {
	if w == nil {
		return 0
	}
	if w.cu != cu {
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
		for i := range g.Waves {
			w := &g.Waves[i]
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
}

// Completion releases scheduling resources immediately, but fetch-ahead replies
// may still refer to a completed wave. Retire its canonical values only after
// the last live queue/stage reference has disappeared.
func (cu *ComputeUnit) retireGroups() {
	for index := 0; index < len(cu.comp.State.Groups); {
		group := &cu.comp.State.Groups[index]
		live := !group.Completed
		if !live {
			for i := range group.Waves {
				if cu.waveReferenced(group.Waves[i].ID) {
					live = true
					break
				}
			}
		}
		if live {
			index++
			continue
		}
		cu.retireGroup(index)
		// Inspect the group moved into this slot before advancing.
	}
}

func (cu *ComputeUnit) retireGroup(index int) {
	s := &cu.comp.State
	retired := s.Groups[index]
	delete(s.GroupLocations, retired.Raw.UID)
	for i := range retired.Waves {
		delete(s.WaveLocations, retired.Waves[i].ID)
		delete(s.WFTimes, retired.Waves[i].ID)
	}
	last := len(s.Groups) - 1
	if index != last {
		s.Groups[index] = s.Groups[last]
		moved := &s.Groups[index]
		s.GroupLocations[moved.Raw.UID] = index
		for i := range moved.Waves {
			s.WaveLocations[moved.Waves[i].ID] = WaveLocation{Group: index, Index: i}
		}
	}
	s.Groups[last] = WorkGroupState{}
	s.Groups = s.Groups[:last]
	for i := range retired.Waves {
		cu.releaseInstruction(retired.Waves[i].Execution.InstID)
		cu.releaseInstruction(retired.Waves[i].Execution.ToIssueID)
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
