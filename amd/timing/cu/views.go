package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// Wavefront is an ID view. It never retains a pointer into component State.
// Views are created for an operation; every accessor resolves the current State.
type Wavefront struct {
	cu     *ComputeUnit
	UID    uint64
	handle uint64
}

// WorkGroup is an ID view of the canonical group in component State.
type WorkGroup struct {
	cu   *ComputeUnit
	UID  uint64
	slot int
}

func slotIndex(handle uint64) int { return int(uint32(handle)) - 1 }
func waveHandle(index int, generation uint32) uint64 {
	return uint64(generation)<<32 | uint64(index+1)
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
func (wf *Wavefront) codeObject() *insts.KernelCodeObject      { return wf.group().codeObject() }
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
func (wg *WorkGroup) codeObject() *insts.KernelCodeObject {
	v := wg.raw().CodeObject
	if !v.Present {
		return nil
	}
	// This operation-local projection shares immutable code bytes, without copying
	// the executable on every fetch eligibility check.
	o := &insts.KernelCodeObject{Data: v.Data, Version: v.Version}
	if v.HasMeta {
		o.KernelCodeObjectMeta = &v.Meta
	}
	if v.HasSymbol {
		o.Symbol = &v.Symbol
	}
	return o
}
func (wg *WorkGroup) waves() []*Wavefront {
	s := wg.state()
	out := make([]*Wavefront, len(s.Waves))
	for i := range s.Waves {
		out[i] = wg.cu.wave(s.Waves[i])
	}
	return out
}

// Inst return the instruction that is being simulated
func (wf *Wavefront) Inst() *insts.Inst {
	i := wf.DynamicInst()
	if i == nil {
		return nil
	}
	return i.Inst
}

// DynamicInst returns the insts with an ID
func (wf *Wavefront) DynamicInst() *wavefront.Inst {
	return wf.cu.instruction(wf.execution().InstID)
}

// SetDynamicInst sets the dynamic inst to execute
func (wf *Wavefront) SetDynamicInst(i *wavefront.Inst) {
	old := wf.execution().InstID
	wf.cu.rememberInstruction(i)
	wf.execution().InstID = instID(i)
	wf.cu.releaseInstruction(old)
}

// ManagedInst returns the wrapped Inst
func (wf *Wavefront) ManagedInst() *wavefront.Inst {
	return wf.cu.instruction(wf.execution().InstID)
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

// NextInst returns the decoded instruction awaiting issue.
func (wf *Wavefront) NextInst() *wavefront.Inst { return wf.cu.instruction(wf.execution().ToIssueID) }

// SetNextInst records the instruction awaiting issue.
func (wf *Wavefront) SetNextInst(i *wavefront.Inst) {
	old := wf.execution().ToIssueID
	wf.cu.rememberInstruction(i)
	wf.execution().ToIssueID = instID(i)
	wf.cu.releaseInstruction(old)
}

// issueNext transfers canonical instruction ownership without re-registering
// or decoding it. The previous instruction may still belong to memory traffic.
func (wf *Wavefront) issueNext() {
	state := wf.execution()
	old := state.InstID
	state.InstID, state.ToIssueID = state.ToIssueID, 0
	wf.cu.releaseInstruction(old)
}
