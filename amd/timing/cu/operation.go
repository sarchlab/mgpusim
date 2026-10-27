package cu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
	"log"
	"math"
)

// operationWave is a call-local adapter. ALUs and coalescers must not retain it.
// It resolves the slot once, then accesses current State directly across lanes.
type operationWave struct {
	cu          *ComputeUnit
	value       *wavefront.ExecutionState
	instruction *insts.Inst
	view        *Wavefront
}

func (wf *Wavefront) operation() *operationWave {
	return &operationWave{cu: wf.cu, value: wf.execution(), view: wf}
}
func (wf *Wavefront) emulation() *operationWave {
	op := wf.operation()
	op.instruction = wf.cu.instruction(op.value.InstID)
	return op
}
func (wf *operationWave) Inst() *insts.Inst { return wf.instruction }
func (wf *operationWave) regAccessor() *operationRegAccessor {
	return &operationRegAccessor{CU: wf.cu, WF: wf}
}

// PID returns pid
func (wf *operationWave) PID() vm.PID {
	return wf.value.PID
}

// SetPID sets pid
func (wf *operationWave) SetPID(pid vm.PID) {
	wf.value.PID = pid
}

// PC returns the program counter
func (wf *operationWave) PC() uint64 {
	return wf.value.PC
}

// SetPC sets the program counter
func (wf *operationWave) SetPC(v uint64) {
	wf.value.PC = v
}

// EXEC returns the exec mask
func (wf *operationWave) EXEC() uint64 {
	return wf.value.EXEC
}

// SetEXEC sets the exec mask
func (wf *operationWave) SetEXEC(v uint64) {
	wf.value.EXEC = v
}

// VCC returns the vector condition code
func (wf *operationWave) VCC() uint64 {
	return wf.value.VCC
}

// SetVCC sets the vector condition code
func (wf *operationWave) SetVCC(v uint64) {
	wf.value.VCC = v
}

// SCC returns the scalar condition code
func (wf *operationWave) SCC() byte {
	return wf.value.SCC
}

// SetSCC sets the scalar condition code
func (wf *operationWave) SetSCC(v byte) {
	wf.value.SCC = v
}

// ReadOperand reads the value of an operand using the RegFileAccessor
func (wf *operationWave) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.value.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.value.VRegOffset
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
func (wf *operationWave) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	numBytes := operand.Register.ByteSize
	if operand.RegCount >= 2 {
		numBytes *= operand.RegCount
	}

	waveOffset := wf.value.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.value.VRegOffset
	}

	data := insts.Uint64ToBytes(value)
	wf.regAccessor().WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data[:numBytes])
}

// ReadOperandBytes reads the raw bytes of an operand
func (wf *operationWave) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.value.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.value.VRegOffset
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
func (wf *operationWave) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	waveOffset := wf.value.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.value.VRegOffset
	}

	wf.regAccessor().WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data)
}

func (wf *operationWave) InstID() uint64 { return wf.value.InstID }
