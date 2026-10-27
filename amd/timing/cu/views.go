package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type waveHandle = wavefront.Handle

// Wavefront is an ID view. It never retains a pointer into component State.
// Views are created for an operation; every accessor resolves the current State.
type Wavefront struct {
	cu     *ComputeUnit
	UID    uint64
	handle waveHandle
}

// WorkGroup is an ID view of the canonical group in component State.
type WorkGroup struct {
	cu   *ComputeUnit
	UID  uint64
	slot int
}

func slotIndex(handle waveHandle) int { return int(uint32(handle)) - 1 }
func makeWaveHandle(index int, generation uint32) waveHandle {
	return waveHandle(generation)<<32 | waveHandle(index+1)
}
func (wf *Wavefront) state() *WaveState {
	s := &wf.cu.comp.State.Waves[slotIndex(wf.handle)]
	if s.ID != wf.UID || s.Generation != uint32(wf.handle>>32) || s.ID == 0 {
		panic(fmt.Sprintf("cu: stale wavefront %d", wf.UID))
	}
	return s
}
func (wf *Wavefront) execution() *wavefront.ExecutionState { return &wf.state().Execution }
func (wf *Wavefront) raw() *kernels.WavefrontValue {
	s := wf.state()
	return &wf.cu.comp.State.Groups[s.Group].Raw.Wavefronts[s.Index]
}
func (wf *Wavefront) group() *WorkGroup {
	index := wf.state().Group
	return &WorkGroup{cu: wf.cu, UID: wf.cu.comp.State.Groups[index].Raw.UID, slot: index}
}
func (wf *Wavefront) packet() *kernels.HsaKernelDispatchPacket { return wf.group().packet() }

// codeObject borrows canonical metadata for the immediate operation only.
// Callers must neither mutate it nor retain it across calls or State replacement.
func (wf *Wavefront) codeObject() *insts.CodeObjectValue {
	group := wf.state().Group
	value := &wf.cu.comp.State.Groups[group].Raw.CodeObject
	if !value.Present || !value.HasMeta {
		panic("cu: wave execution requires code-object metadata")
	}
	return value
}
func (wf *Wavefront) scoreboard() *Scoreboard {
	s := wf.state()
	if !s.HasScoreboard {
		return nil
	}
	return &s.Scoreboard
}
func (wf *Wavefront) regAccessor() *CURegFileAccessor { return &CURegFileAccessor{CU: wf.cu, WF: wf} }
func (wg *WorkGroup) state() *WorkGroupState {
	s := &wg.cu.comp.State.Groups[wg.slot]
	if !s.Occupied || s.Raw.UID != wg.UID {
		panic("cu: stale work-group")
	}
	return s
}
func (wg *WorkGroup) raw() *kernels.WorkGroupValue { return &wg.state().Raw }
func (wg *WorkGroup) packet() *kernels.HsaKernelDispatchPacket {
	p := wg.raw().Packet
	if !p.Present {
		return nil
	}
	return &p.Packet
}

func (wg *WorkGroup) waves() []*Wavefront {
	s := wg.state()
	out := make([]*Wavefront, len(s.Waves))
	for i := range s.Waves {
		out[i] = wg.cu.wave(s.Waves[i])
	}
	return out
}

// Inst returns the shared read-only description of the current instruction.
func (wf *Wavefront) Inst() *insts.Inst { return wf.cu.instruction(wf.InstID()) }

// InstID is the dynamic instruction identity, distinct from its description.
func (wf *Wavefront) InstID() uint64 { return wf.execution().InstID }

// SetInstID selects an instruction already admitted to canonical State.
func (wf *Wavefront) SetInstID(id uint64) {
	wf.requireInstruction(id)
	old := wf.execution().InstID
	wf.execution().InstID = id
	wf.cu.releaseInstruction(old)
}

func (wf *Wavefront) PID() vm.PID       { return wf.operation().PID() }
func (wf *Wavefront) SetPID(pid vm.PID) { wf.operation().SetPID(pid) }
func (wf *Wavefront) PC() uint64        { return wf.operation().PC() }
func (wf *Wavefront) SetPC(v uint64)    { wf.operation().SetPC(v) }
func (wf *Wavefront) EXEC() uint64      { return wf.operation().EXEC() }
func (wf *Wavefront) SetEXEC(v uint64)  { wf.operation().SetEXEC(v) }
func (wf *Wavefront) VCC() uint64       { return wf.operation().VCC() }
func (wf *Wavefront) SetVCC(v uint64)   { wf.operation().SetVCC(v) }
func (wf *Wavefront) SCC() byte         { return wf.operation().SCC() }
func (wf *Wavefront) SetSCC(v byte)     { wf.operation().SetSCC(v) }
func (wf *Wavefront) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	return wf.operation().ReadOperand(operand, laneID)
}
func (wf *Wavefront) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	wf.operation().WriteOperand(operand, laneID, value)
}
func (wf *Wavefront) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	return wf.operation().ReadOperandBytes(operand, laneID, byteCount)
}
func (wf *Wavefront) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	wf.operation().WriteOperandBytes(operand, laneID, data)
}

// NextInst returns the shared read-only description awaiting issue.
func (wf *Wavefront) NextInst() *insts.Inst { return wf.cu.instruction(wf.execution().ToIssueID) }

// SetNextInstID selects an instruction already admitted to canonical State.
func (wf *Wavefront) SetNextInstID(id uint64) {
	wf.requireInstruction(id)
	old := wf.execution().ToIssueID
	wf.execution().ToIssueID = id
	wf.cu.releaseInstruction(old)
}

func (wf *Wavefront) requireInstruction(id uint64) {
	if id == 0 {
		return
	}
	instruction, exists := wf.cu.comp.State.Instructions[id]
	if !exists || instruction.Owner != wf.handle {
		panic("cu: instruction is not admitted for this wave")
	}
}

// issueNext transfers canonical instruction ownership without re-registering
// or decoding it. The previous instruction may still belong to memory traffic.
func (wf *Wavefront) issueNext() {
	state := wf.execution()
	old := state.InstID
	state.InstID, state.ToIssueID = state.ToIssueID, 0
	wf.cu.releaseInstruction(old)
}
