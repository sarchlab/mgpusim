package emu

import (
	"log"
	"sync"

	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

type VectorInstWorker struct {
	CU gcn3.ComputeUnit
}

func NewVectorInstWorker() *VectorInstWorker {
	return new(VectorInstWorker)
}

// Run execute an instruction for a wavefront. The wiFlatID should be the
// flattened ID of the first workitem in a wavefront.
func (w *VectorInstWorker) Run(inst *disasm.Instruction, wiFlatID int) error {
	switch inst.FormatType {
	case disasm.Vop2:
		return w.runVop2(inst, wiFlatID)
	case disasm.Vop1:
		return w.runVop1(inst, wiFlatID)
	default:
		log.Panicf("instruction format %s is not supported", inst.FormatName)
	}
	return nil
}

type instFunc func(inst *disasm.Instruction, wiFlatID int,
	waitGroup *sync.WaitGroup) error

func (w *VectorInstWorker) runForActiveWI(
	f instFunc,
	inst *disasm.Instruction,
	wiFlatID int,
) error {
	exec := w.getRegUint64(disasm.Regs[disasm.Exec], wiFlatID)
	waitGroup := new(sync.WaitGroup)
	for i := 0; i < 64; i++ {
		mask := uint64(1) << uint(i)
		if exec&mask != 0 {
			waitGroup.Add(1)
			go f(inst, int(wiFlatID+i), waitGroup)
		}
	}

	pc := w.getRegUint64(disasm.Regs[disasm.Pc], wiFlatID)
	pc += uint64(inst.ByteSize)
	w.putRegUint64(disasm.Regs[disasm.Pc], wiFlatID, pc)

	waitGroup.Wait()

	return nil
}

func (w *VectorInstWorker) runVop2(inst *disasm.Instruction,
	wiFlatID int) error {
	switch inst.Opcode {
	default:
		log.Panicf("opcode %d of Sop2 format is not supported", inst.Opcode)
	}
	return nil
}

func (w *VectorInstWorker) runVop1(inst *disasm.Instruction,
	wiFlatID int) error {
	switch inst.Opcode {
	case 1:
		w.runForActiveWI(w.runVMovB32, inst, wiFlatID)
	default:
		log.Panicf("opcode %d of Sop2 format is not supported", inst.Opcode)
	}
	return nil
}

func (w *VectorInstWorker) runVMovB32(inst *disasm.Instruction,
	wiFlatID int, waitGroup *sync.WaitGroup) error {
	defer waitGroup.Done()
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)
	w.putRegUint32(inst.Dst.Register, wiFlatID, src0Value)
	return nil
}

func (w *VectorInstWorker) getRegUint64(
	reg *disasm.Reg, wiFlatID int,
) uint64 {
	data := w.CU.ReadReg(reg, wiFlatID, 8)
	return disasm.BytesToUint64(data)
}

func (w *VectorInstWorker) getRegUint32(
	reg *disasm.Reg, wiFlatID int,
) uint32 {
	data := w.CU.ReadReg(reg, wiFlatID, 4)
	return disasm.BytesToUint32(data)
}

func (w *VectorInstWorker) getRegUint8(
	reg *disasm.Reg, wiFlatID int,
) uint8 {
	data := w.CU.ReadReg(reg, wiFlatID, 1)
	return disasm.BytesToUint8(data)
}

func (w *VectorInstWorker) getOperandValueUint32(
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

func (w *VectorInstWorker) putRegUint8(
	reg *disasm.Reg, wiFlatID int, value uint8,
) {
	data := disasm.Uint8ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *VectorInstWorker) putRegUint32(
	reg *disasm.Reg, wiFlatID int, value uint32,
) {
	data := disasm.Uint32ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *VectorInstWorker) putRegUint64(
	reg *disasm.Reg, wiFlatID int, value uint64,
) {
	data := disasm.Uint64ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}
