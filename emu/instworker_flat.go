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

			req, err := w.CU.ReadMem(addr, 2, info, now)

			if err != nil {
				return nil
			}

			wf.WIMemRequested[i] = true
			wf.MemAccess = append(wf.MemAccess, req)
		}
		mask = mask << 1
	}

	// The commit process
	if wf.IsAllMemAccessReady() {
		wf.MemAccess = make([]*mem.AccessReq, 0, 64)
		wf.WIMemRequested = make([]bool, 64)
		pc := w.getRegUint64(disasm.Regs[disasm.Pc], wf.Wf.FirstWiFlatID)
		pc += uint64(inst.ByteSize)
		w.putRegUint64(disasm.Regs[disasm.Pc], wf.Wf.FirstWiFlatID, pc)
		w.Scheduler.Completed(wf)
	}

	return nil
}
