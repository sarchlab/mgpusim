package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/mem"
)

func (w *InstWorkerImpl) runFlat(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	switch inst.Opcode {
	case 18:
		return w.runFlatLoadUShort(wf, now)
	default:
		log.Panicf("instruction opcode %d for type flat not supported\n",
			inst.Opcode)
	}
	return nil
}

func (w *InstWorkerImpl) continueFlat(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	switch inst.Opcode {
	case 18:
		return w.continueFlatLoadUShort(wf, now)
	default:
		log.Panicf("instruction opcode %d for type flat not supported\n",
			inst.Opcode)
	}
	return nil
}

func (w *InstWorkerImpl) runFlatLoadUShort(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	exec := w.getRegUint64(disasm.Regs[disasm.Exec], wf.Wf.FirstWiFlatID)
	mask := uint64(1)

	// The request process
	for i := 0; i < 64; i++ {
		if wf.WIMemRequested[i] == false && exec&mask != 0 {
			wiFlatID := wf.Wf.FirstWiFlatID + i
			addr := w.getRegUint64(inst.Addr.Register, wiFlatID)

			info := new(MemAccessInfo)
			info.wiFlatID = wiFlatID
			info.RegToSet = inst.Dst.Register
			info.WfScheduleInfo = wf

			req := mem.NewAccessReq()
			req.Address = addr
			req.ByteSize = 2
			req.Info = info
			req.SetSendTime(now)
			req.SetSource(w.CU)
			req.SetDestination(w.DataMem)

			error := w.ToDataMem.Send(req)
			if error != nil && error.Recoverable == false {
				log.Panic(error)
			} else if error != nil {
				// Leave the look and the scheduler will schedule next cycle
				// to retry
				break
			} else {
				wf.WIMemRequested[i] = true
				wf.MemAccess = append(wf.MemAccess, req)
			}
		}
		mask = mask << 1
	}

	// The commit process

	return nil
}

func (w *InstWorkerImpl) continueFlatLoadUShort(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	w.Scheduler.Completed(wf)
	return nil
}
