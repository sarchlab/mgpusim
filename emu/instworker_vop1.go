package emu

import (
	"log"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

type instFunc func(
	wf *WfScheduleInfo,
	wiFlatID int,
	now core.VTimeInSec,
	wg *sync.WaitGroup,
) error

func (w *InstWorkerImpl) runVop1(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	switch inst.Opcode {
	case 1:
		w.runForActiveWI(w.runVMovB32, wf, now)
	default:
		log.Panicf("instruction opcode %d for type vop1 not supported\n",
			inst.Opcode)
	}
	return nil
}

func (w *InstWorkerImpl) runForActiveWI(
	f instFunc,
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	exec := w.getRegUint64(insts.Regs[insts.Exec], wf.Wf.FirstWiFlatID)
	waitGroup := new(sync.WaitGroup)
	for i := 0; i < 64; i++ {
		mask := uint64(1) << uint(i)
		if exec&mask != 0 {
			waitGroup.Add(1)
			go f(wf, int(wf.Wf.FirstWiFlatID+i), now, waitGroup)
		}
	}
	w.IncreasePc(wf, inst.ByteSize)
	w.Scheduler.Completed(wf)
	waitGroup.Wait()
	return nil
}

func (w *InstWorkerImpl) runVMovB32(
	wf *WfScheduleInfo,
	wiFlatID int,
	now core.VTimeInSec,
	waitGroup *sync.WaitGroup,
) error {
	defer waitGroup.Done()
	inst := wf.Inst
	src0Value := w.getOperandValueUint32(inst.Src0, wiFlatID)
	w.putRegUint32(inst.Dst.Register, wiFlatID, src0Value)
	return nil
}
