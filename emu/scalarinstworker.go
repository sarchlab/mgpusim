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
	case 4:
		return w.runSAddCU32(inst, wiFlatID)
	default:
		log.Panicf("opcode %d of Sop2 format is not supported", inst.Opcode)
	}
	return nil
}

func (w *ScalarInstWorker) runSAddU32(inst *disasm.Instruction,
	wiFlatID int) error {

	pc := w.getRegUint64(disasm.Regs[disasm.Pc], wiFlatID)
	src1Value := w.getOperandValueUint32(inst.Src1, wiFlatID)
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)

	// Overflow check
	var sccValue uint8
	if src1Value&(1<<31) != 0 && src0Value&(1<<31) != 0 {
		sccValue = 1
	}

	dstValue := src0Value + src1Value
	pc += uint64(inst.ByteSize)

	w.putRegUint32(inst.Dst.Register, wiFlatID, dstValue)
	w.putRegUint64(disasm.Regs[disasm.Pc], wiFlatID, pc)
	w.putRegUint8(disasm.Regs[disasm.Scc], wiFlatID, sccValue)

	return nil
}

func (w *ScalarInstWorker) runSAddCU32(inst *disasm.Instruction,
	wiFlatID int) error {
	pc := w.getRegUint64(disasm.Regs[disasm.Pc], wiFlatID)
	src1Value := w.getOperandValueUint32(inst.Src1, wiFlatID)
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)

	// Overflow check
	sccValue := w.getRegUint8(disasm.Regs[disasm.Scc], wiFlatID)

	dstValue := src0Value + src1Value + uint32(sccValue)
	if src1Value&(1<<31) != 0 && src0Value&(1<<31) != 0 {
		sccValue = 1
	}
	pc += uint64(inst.ByteSize)

	w.putRegUint32(inst.Dst.Register, wiFlatID, dstValue)
	w.putRegUint64(disasm.Regs[disasm.Pc], wiFlatID, pc)
	w.putRegUint8(disasm.Regs[disasm.Scc], wiFlatID, sccValue)

	return nil

}

func (w *ScalarInstWorker) getRegUint64(
	reg *disasm.Reg, wiFlatID int,
) uint64 {
	data := w.CU.ReadReg(reg, wiFlatID, 8)
	return disasm.BytesToUint64(data)
}

func (w *ScalarInstWorker) getRegUint32(
	reg *disasm.Reg, wiFlatID int,
) uint32 {
	data := w.CU.ReadReg(reg, wiFlatID, 4)
	return disasm.BytesToUint32(data)
}

func (w *ScalarInstWorker) getRegUint8(
	reg *disasm.Reg, wiFlatID int,
) uint8 {
	data := w.CU.ReadReg(reg, wiFlatID, 1)
	return disasm.BytesToUint8(data)
}

func (w *ScalarInstWorker) getOperandValueUint32(
	operand *disasm.Operand, wiFlatID int,
) uint32 {
	switch operand.OperandType {
	case disasm.RegOperand:
		return w.getRegUint32(operand.Register, wiFlatID)
	case disasm.IntOperand:
		return uint32(operand.IntValue)
	case disasm.LiteralConstant:
		return uint32(operand.LiteralConstant)
	default:
		log.Panic("invalid operand type")
	}
	return 0
}

func (w *ScalarInstWorker) putRegUint8(
	reg *disasm.Reg, wiFlatID int, value uint8,
) {
	data := disasm.Uint8ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *ScalarInstWorker) putRegUint32(
	reg *disasm.Reg, wiFlatID int, value uint32,
) {
	data := disasm.Uint32ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *ScalarInstWorker) putRegUint64(
	reg *disasm.Reg, wiFlatID int, value uint64,
) {
	data := disasm.Uint64ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}
