package wavefront

import (
	"log"
	"math"
	"sync"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// RegFileAccessor provides access to register files for a wavefront.
type RegFileAccessor interface {
	ReadReg(reg *insts.Reg, regCount int, laneID int, waveOffset int) []byte
	WriteReg(reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte)
}

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	WfDispatching      WfState = iota // Dispatching in progress, not ready to run
	WfReady                           // Allow the scheduler to schedule instruction
	WfRunning                         // Instruction in fight
	WfCompleted                       // Wavefront completed
	WfAtBarrier                       // Wavefront at barrier
	WfSampledCompleted                // Wavefront completed at Sampling
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront
	sync.RWMutex

	WG *WorkGroup

	*ExecutionState
	resolveInst       func(uint64) *Inst
	rememberInst      func(*Inst)
	releaseInst       func(uint64)
	localInstructions map[uint64]*Inst

	RegAccessor RegFileAccessor

	// ScoreboardData holds per-wavefront register scoreboard state.
	// When register scoreboard is enabled, this contains a *cu.Scoreboard
	// (stored as interface{} to avoid circular imports).
	ScoreboardData interface{}
}

// NewWavefront creates a new Wavefront of the timing package, wrapping the
// wavefront from the kernels package.
func NewWavefront(raw *kernels.Wavefront) *Wavefront {
	wf := &Wavefront{ExecutionState: &ExecutionState{}, localInstructions: make(map[uint64]*Inst)}
	wf.Wavefront = raw

	wf.InstBuffer = make([]byte, 0, 256)

	return wf
}

// Inst return the instruction that is being simulated
func (wf *Wavefront) Inst() *insts.Inst {
	if wf.DynamicInst() == nil {
		return nil
	}
	return wf.DynamicInst().Inst
}

// DynamicInst returns the insts with an ID
func (wf *Wavefront) DynamicInst() *Inst {
	return wf.instruction(wf.InstID)
}

// SetDynamicInst sets the dynamic inst to execute
func (wf *Wavefront) SetDynamicInst(i *Inst) {
	old := wf.InstID
	wf.recordInstruction(i)
	wf.InstID = instructionID(i)
	wf.releaseInstruction(old)
}

// ManagedInst returns the wrapped Inst
func (wf *Wavefront) ManagedInst() *Inst {
	return wf.instruction(wf.InstID)
}

// PID returns pid
func (wf *Wavefront) PID() vm.PID {
	return wf.ExecutionState.PID
}

// SetPID sets pid
func (wf *Wavefront) SetPID(pid vm.PID) {
	wf.ExecutionState.PID = pid
}

// PC returns the program counter
func (wf *Wavefront) PC() uint64 {
	return wf.ExecutionState.PC
}

// SetPC sets the program counter
func (wf *Wavefront) SetPC(v uint64) {
	wf.ExecutionState.PC = v
}

// EXEC returns the exec mask
func (wf *Wavefront) EXEC() uint64 {
	return wf.ExecutionState.EXEC
}

// SetEXEC sets the exec mask
func (wf *Wavefront) SetEXEC(v uint64) {
	wf.ExecutionState.EXEC = v
}

// VCC returns the vector condition code
func (wf *Wavefront) VCC() uint64 {
	return wf.ExecutionState.VCC
}

// SetVCC sets the vector condition code
func (wf *Wavefront) SetVCC(v uint64) {
	wf.ExecutionState.VCC = v
}

// SCC returns the scalar condition code
func (wf *Wavefront) SCC() byte {
	return wf.ExecutionState.SCC
}

// SetSCC sets the scalar condition code
func (wf *Wavefront) SetSCC(v byte) {
	wf.ExecutionState.SCC = v
}

// ReadOperand reads the value of an operand using the RegFileAccessor
func (wf *Wavefront) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.VRegOffset
		}
		buf := wf.RegAccessor.ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
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

	waveOffset := wf.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.VRegOffset
	}

	data := insts.Uint64ToBytes(value)
	wf.RegAccessor.WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data[:numBytes])
}

// ReadOperandBytes reads the raw bytes of an operand
func (wf *Wavefront) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.VRegOffset
		}
		buf := wf.RegAccessor.ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
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

	waveOffset := wf.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.VRegOffset
	}

	wf.RegAccessor.WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data)
}

// BindExecution makes the view refer directly to its owner's canonical values.
func (wf *Wavefront) BindExecution(
	s *ExecutionState, resolve func(uint64) *Inst, remember func(*Inst), release func(uint64),
) {
	wf.ExecutionState = s
	wf.resolveInst = resolve
	wf.rememberInst = remember
	wf.releaseInst = release
	wf.localInstructions = nil
}
func (wf *Wavefront) instruction(id uint64) *Inst {
	if id == 0 {
		return nil
	}
	if wf.resolveInst != nil {
		return wf.resolveInst(id)
	}
	return wf.localInstructions[id]
}
func instructionID(i *Inst) uint64 {
	if i == nil {
		return 0
	}
	return i.ID
}
func (wf *Wavefront) recordInstruction(i *Inst) {
	if i == nil {
		return
	}
	if wf.rememberInst != nil {
		wf.rememberInst(i)
		return
	}
	wf.localInstructions[i.ID] = i
}

// NextInst is the decoded instruction awaiting issue.
func (wf *Wavefront) NextInst() *Inst { return wf.instruction(wf.ToIssueID) }

// SetNextInst records the instruction awaiting issue.
func (wf *Wavefront) SetNextInst(i *Inst) {
	old := wf.ToIssueID
	wf.recordInstruction(i)
	wf.ToIssueID = instructionID(i)
	wf.releaseInstruction(old)
}
func (wf *Wavefront) releaseInstruction(id uint64) {
	if id == 0 {
		return
	}
	if wf.releaseInst != nil {
		wf.releaseInst(id)
		return
	}
	if id != wf.InstID && id != wf.ToIssueID {
		delete(wf.localInstructions, id)
	}
}
