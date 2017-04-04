package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
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
	log.Printf("%.10f: Inst %s\n", now, wf.Inst.String())
	inst := wf.Inst
	switch inst.FormatType {
	case insts.Sop2:
		return w.runSop2(wf, now)
	case insts.Vop1:
		return w.runVop1(wf, now)
	case insts.Flat:
		return w.runFlat(wf, now)
	default:
		log.Panicf("instruction type %s not supported\n", inst.FormatName)
	}
	return nil
}

// IncreasePC increase PC by a certain amount
func (w *InstWorkerImpl) IncreasePC(wf *WfScheduleInfo, amount int) {
	pc := w.getRegUint64(insts.Regs[insts.Pc], wf.Wf.FirstWiFlatID)
	pc += uint64(amount)
	w.putRegUint64(insts.Regs[insts.Pc], wf.Wf.FirstWiFlatID, pc)
}

func (w *InstWorkerImpl) getRegUint64(
	reg *insts.Reg, wiFlatID int,
) uint64 {
	data := w.CU.ReadReg(reg, wiFlatID, 8)
	return insts.BytesToUint64(data)
}

func (w *InstWorkerImpl) getRegUint32(
	reg *insts.Reg, wiFlatID int,
) uint32 {
	data := w.CU.ReadReg(reg, wiFlatID, 4)
	return insts.BytesToUint32(data)
}

func (w *InstWorkerImpl) getRegUint8(
	reg *insts.Reg, wiFlatID int,
) uint8 {
	data := w.CU.ReadReg(reg, wiFlatID, 1)
	return insts.BytesToUint8(data)
}

func (w *InstWorkerImpl) getOperandValueUint32(
	operand *insts.Operand, wiFlatID int,
) uint32 {
	switch operand.OperandType {
	case insts.RegOperand:
		return w.getRegUint32(operand.Register, wiFlatID)
	case insts.IntOperand:
		return uint32(operand.IntValue)
	case insts.LiteralConstant:
		return uint32(operand.LiteralConstant)
	default:
		log.Panic("invalid operand type")
	}
	return 0
}

func (w *InstWorkerImpl) putRegUint8(
	reg *insts.Reg, wiFlatID int, value uint8,
) {
	data := insts.Uint8ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *InstWorkerImpl) putRegUint32(
	reg *insts.Reg, wiFlatID int, value uint32,
) {
	data := insts.Uint32ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}

func (w *InstWorkerImpl) putRegUint64(
	reg *insts.Reg, wiFlatID int, value uint64,
) {
	data := insts.Uint64ToBytes(value)
	w.CU.WriteReg(reg, wiFlatID, data)
}
