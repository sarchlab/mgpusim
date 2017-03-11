package emu

import (
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
	switch inst.FormatType {
	case disasm.Sop2:
		return w.runSop2(inst, wiFlatID)
	default:
		log.Panicf("instruction format %s is not supported",
			inst.FormatName)
	}
	return nil
}

func (w *ScalarInstWorker) runSop2(inst *disasm.Instruction,
	wiFlatID int) error {
	switch inst.Opcode {
	case 0:
		return w.runSAddU32(inst, wiFlatID)
	default:
		log.Panicf("opcode %d of Sop2 format is not supported", inst.Opcode)
	}
	return nil
}

func (w *ScalarInstWorker) runSAddU32(inst *disasm.Instruction,
	wiFlatID int) error {

	pc := w.getRegisterValueUint64(disasm.Regs[disasm.Pc], wiFlatID)
	src1Value := w.getOperandValueUint32(inst.Src1, wiFlatID)
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)
	dstValue := src0Value + src1Value
	pc += uint64(inst.ByteSize)
	w.putRegisterValueUint32(inst.Dst.Register, wiFlatID, dstValue)
	w.putRegisterValueUint64(disasm.Regs[disasm.Pc], wiFlatID, pc)
	return nil
}

func (w *ScalarInstWorker) getRegisterValueUint64(
	reg *disasm.Reg, wiFlatID int,
) uint64 {
	data := w.CU.ReadReg(reg, wiFlatID, 8)
	return disasm.BytesToUint64(data)
}

func (w *ScalarInstWorker) getRegisterValueUint32(
	reg *disasm.Reg, wiFlatID int,
) uint32 {
	data := w.CU.ReadReg(reg, wiFlatID, 4)
	return disasm.BytesToUint32(data)
}

func (w *ScalarInstWorker) getOperandValueUint32(
	operand *disasm.Operand, wiFlatID int,
) uint32 {
	switch operand.OperandType {
	case disasm.RegOperand:
		return w.getRegisterValueUint32(operand.Register, wiFlatID)
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
	reg *disasm.Reg, wiFlatID int, value uint32,
) {
	data := disasm.Uint32ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *ScalarInstWorker) putRegisterValueUint64(
	reg *disasm.Reg, wiFlatID int, value uint64,
) {
	data := disasm.Uint64ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}
