package emu

import (
	"encoding/binary"
	"log"

	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// ScalarInstWorker defines how the scalar instructions are executed.
type ScalarInstWorker struct {
	CU gcn3.ComputeUnit
}

// NewScalarInstWorker returns a newly created ScalarInstWorker
func NewScalarInstWorker() *ScalarInstWorker {
	return new(ScalarInstWorker)
}

// Run execute an instruction for a wavefront
func (w *ScalarInstWorker) Run(inst *disasm.Instruction, wiFlatID int) error {
	log.Print(wiFlatID, inst)
	switch inst.FormatType {
	case disasm.Sop2:
		return w.runSop2(inst, wiFlatID)
	default:
		log.Panicf("instruction format %s is not supported",
			inst.FormatName)
	}
	return nil
}

func (w *ScalarInstWorker) runSop2(inst *disasm.Instruction, wiFlatID int) error {
	switch inst.Opcode {
	case 0:
		return w.runSAddU32(inst, wiFlatID)
	default:
		log.Panicf("opcode %d of Sop2 format is not supported", inst.Opcode)
	}
	return nil
}

func (w *ScalarInstWorker) runSAddU32(inst *disasm.Instruction, wiFlatID int) error {
	src1Value := w.getOperandValueUint32(inst.Src1, wiFlatID)
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)
	dstValue := src0Value + src1Value
	w.putRegisterValueUint32(inst.Dst, wiFlatID, dstValue)
	return nil
}

func (w *ScalarInstWorker) getOperandValueUint32(
	operand *disasm.Operand, wiFlatID int) uint32 {
	switch operand.OperandType {
	case disasm.RegOperand:
		data := w.CU.ReadReg(operand.Register, wiFlatID, 4)
		return binary.LittleEndian.Uint32(data)
	case disasm.IntOperand:
		return uint32(operand.IntValue)
	case disasm.LiteralConstant:
		return uint32(operand.LiteralConstant)
	default:
		log.Panic("invalid operand type")
	}
	return 0
}

func (w *ScalarInstWorker) putRegisterValueUint32(
	operand *disasm.Operand, wiFlatID int, value uint32) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, value)
	w.CU.WriteReg(operand.Register, wiFlatID, data)
}
