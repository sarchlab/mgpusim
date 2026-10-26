package cu

import (
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// Wavefront is an ID view. It never retains a pointer into component State.
// Views are created for an operation; every accessor resolves the current State.
type Wavefront struct {
	cu  *ComputeUnit
	UID uint64
}

// WorkGroup is an ID view of the canonical group in component State.
type WorkGroup struct {
	cu  *ComputeUnit
	UID uint64
}

// WaveLocation is a checkpointed index into the canonical work-group wave array.
type WaveLocation struct {
	Group int
	Index int
}

func (wf *Wavefront) location() WaveLocation {
	loc, ok := wf.cu.comp.State.WaveLocations[wf.UID]
	if !ok {
		panic(fmt.Sprintf("cu: unknown wavefront %d", wf.UID))
	}
	return loc
}
func (wf *Wavefront) state() *WaveState {
	loc := wf.location()
	return &wf.cu.comp.State.Groups[loc.Group].Waves[loc.Index]
}
func (wf *Wavefront) execution() *wavefront.ExecutionState { return &wf.state().Execution }
func (wf *Wavefront) raw() *kernels.WavefrontValue {
	loc := wf.location()
	return &wf.cu.comp.State.Groups[loc.Group].Raw.Wavefronts[loc.Index]
}
func (wf *Wavefront) group() *WorkGroup {
	return &WorkGroup{cu: wf.cu, UID: wf.cu.comp.State.Groups[wf.location().Group].Raw.UID}
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
func (cu *ComputeUnit) groupState(id uint64) *WorkGroupState {
	index, ok := cu.comp.State.GroupLocations[id]
	if !ok {
		panic(fmt.Sprintf("cu: unknown work-group %d", id))
	}
	return &cu.comp.State.Groups[index]
}
func (wg *WorkGroup) state() *WorkGroupState       { return wg.cu.groupState(wg.UID) }
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
		out[i] = wg.cu.wave(s.Waves[i].ID)
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

// PID returns pid
func (wf *Wavefront) PID() vm.PID {
	return wf.execution().PID
}

// SetPID sets pid
func (wf *Wavefront) SetPID(pid vm.PID) {
	wf.execution().PID = pid
}

// PC returns the program counter
func (wf *Wavefront) PC() uint64 {
	return wf.execution().PC
}

// SetPC sets the program counter
func (wf *Wavefront) SetPC(v uint64) {
	wf.execution().PC = v
}

// EXEC returns the exec mask
func (wf *Wavefront) EXEC() uint64 {
	return wf.execution().EXEC
}

// SetEXEC sets the exec mask
func (wf *Wavefront) SetEXEC(v uint64) {
	wf.execution().EXEC = v
}

// VCC returns the vector condition code
func (wf *Wavefront) VCC() uint64 {
	return wf.execution().VCC
}

// SetVCC sets the vector condition code
func (wf *Wavefront) SetVCC(v uint64) {
	wf.execution().VCC = v
}

// SCC returns the scalar condition code
func (wf *Wavefront) SCC() byte {
	return wf.execution().SCC
}

// SetSCC sets the scalar condition code
func (wf *Wavefront) SetSCC(v byte) {
	wf.execution().SCC = v
}

// ReadOperand reads the value of an operand using the RegFileAccessor
func (wf *Wavefront) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.execution().SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.execution().VRegOffset
		}
		buf := wf.regAccessor().ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
		if len(buf) < 8 {
			padded := make([]byte, 8)
			copy(padded, buf)
			buf = padded
		}
		return insts.BytesToUint64(buf)
	case insts.IntOperand:
		return uint64(operand.IntValue)
	case insts.FloatOperand:
		return uint64(math.Float32bits(float32(operand.FloatValue)))
	case insts.LiteralConstant:
		return uint64(operand.LiteralConstant)
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return 0
	}
}

// WriteOperand writes a value to an operand using the RegFileAccessor
func (wf *Wavefront) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	numBytes := operand.Register.ByteSize
	if operand.RegCount >= 2 {
		numBytes *= operand.RegCount
	}

	waveOffset := wf.execution().SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.execution().VRegOffset
	}

	data := insts.Uint64ToBytes(value)
	wf.regAccessor().WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data[:numBytes])
}

// ReadOperandBytes reads the raw bytes of an operand
func (wf *Wavefront) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.execution().SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.execution().VRegOffset
		}
		buf := wf.regAccessor().ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
		if len(buf) > byteCount {
			return buf[:byteCount]
		}
		return buf
	case insts.IntOperand:
		data := insts.Uint64ToBytes(uint64(operand.IntValue))
		return data[:byteCount]
	case insts.FloatOperand:
		data := insts.Uint64ToBytes(uint64(math.Float32bits(float32(operand.FloatValue))))
		return data[:byteCount]
	case insts.LiteralConstant:
		data := insts.Uint64ToBytes(uint64(operand.LiteralConstant))
		return data[:byteCount]
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return nil
	}
}

// WriteOperandBytes writes raw bytes to an operand
func (wf *Wavefront) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	waveOffset := wf.execution().SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.execution().VRegOffset
	}

	wf.regAccessor().WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data)
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
