package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A InstWorker is where instructions got executed
type InstWorker interface {
	Run(wf *WfScheduleInfo, now core.VTimeInSec) error
}

// InstWorkerImpl is the standard implmentation of a InstWorker
type InstWorkerImpl struct {
	CU        gcn3.ComputeUnit
	Scheduler *Scheduler
}

// Run will emulate the result of a instruction execution
func (w *InstWorkerImpl) Run(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	log.Printf("%f: Inst %s\n", now, wf.Inst.String())
	inst := wf.Inst
	switch inst.FormatType {
	case disasm.Sop2:
		return w.runSop2(wf, now)
	default:
		log.Panicf("instruction type %s not supported\n", inst.FormatName)
	}
	// w.Scheduler.Completed(wf)
	return nil
}

func (w *InstWorkerImpl) runSop2(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	switch inst.Opcode {
	case 0:
		return w.runSADDU32(wf, now)
	default:
		log.Panicf("instruction opcode %d for type sop2 not supported\n",
			inst.Opcode)
	}
	return nil
}

func (w *InstWorkerImpl) runSADDU32(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	pc := w.getRegUint64(disasm.Regs[disasm.Pc], wf.Wf.FirstWiFlatID)
	src1Value := w.getOperandValueUint32(inst.Src1, wf.Wf.FirstWiFlatID)
	src0Value := w.getOperandValueUint32(inst.Src0, wf.Wf.FirstWiFlatID)
	// Overflow check
	var sccValue uint8
	if src1Value&(1<<31) != 0 && src0Value&(1<<31) != 0 {
		sccValue = 1
	}
	dstValue := src0Value + src1Value
	pc += uint64(inst.ByteSize)
	w.putRegUint32(inst.Dst.Register, wf.Wf.FirstWiFlatID, dstValue)
	w.putRegUint64(disasm.Regs[disasm.Pc], wf.Wf.FirstWiFlatID, pc)
	w.putRegUint8(disasm.Regs[disasm.Scc], wf.Wf.FirstWiFlatID, sccValue)

	w.Scheduler.Completed(wf)
	return nil
}

func (w *InstWorkerImpl) getRegUint64(
	reg *disasm.Reg, wiFlatID int,
) uint64 {
	data := w.CU.ReadReg(reg, wiFlatID, 8)
	return disasm.BytesToUint64(data)
}

func (w *InstWorkerImpl) getRegUint32(
	reg *disasm.Reg, wiFlatID int,
) uint32 {
	data := w.CU.ReadReg(reg, wiFlatID, 4)
	return disasm.BytesToUint32(data)
}

func (w *InstWorkerImpl) getRegUint8(
	reg *disasm.Reg, wiFlatID int,
) uint8 {
	data := w.CU.ReadReg(reg, wiFlatID, 1)
	return disasm.BytesToUint8(data)
}

func (w *InstWorkerImpl) getOperandValueUint32(
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

func (w *InstWorkerImpl) putRegUint8(
	reg *disasm.Reg, wiFlatID int, value uint8,
) {
	data := disasm.Uint8ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *InstWorkerImpl) putRegUint32(
	reg *disasm.Reg, wiFlatID int, value uint32,
) {
	data := disasm.Uint32ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *InstWorkerImpl) putRegUint64(
	reg *disasm.Reg, wiFlatID int, value uint64,
) {
	data := disasm.Uint64ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}
