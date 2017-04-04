package emu

import (
	"log"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
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

func (w *InstWorkerImpl) flatLoadInstFunc(
	wf *WfScheduleInfo,
	wiFlatID int,
	now core.VTimeInSec,
	wg *sync.WaitGroup,
	byteSize int,
) {
	defer wg.Done()

	inst := wf.Inst
	addr := w.getRegUint64(inst.Addr.Register, wiFlatID)

	info := new(MemAccessInfo)
	info.wiFlatID = wiFlatID
	info.RegToSet = inst.Dst.Register
	info.WfScheduleInfo = wf

	req, err := w.CU.ReadMem(addr, byteSize, info, now)

	if err != nil {
		return
	}

	wf.WIMemRequested[wiFlatID-wf.Wf.FirstWiFlatID] = true
	wf.MemAccess[wiFlatID-wf.Wf.FirstWiFlatID] = req
}

func (w *InstWorkerImpl) runFlatLoadUShort(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	inst := wf.Inst
	exec := w.getRegUint64(insts.Regs[insts.Exec], wf.Wf.FirstWiFlatID)
	mask := uint64(1)
	waitGroup := new(sync.WaitGroup)

	// The request process
	for i := 0; i < 64; i++ {
		if wf.WIMemRequested[i] == false && exec&(mask<<uint(i)) != 0 {
			waitGroup.Add(1)
			go w.flatLoadInstFunc(wf, wf.Wf.FirstWiFlatID+i, now, waitGroup, 2)
		}
	}
	waitGroup.Wait()

	// The commit process
	if wf.IsAllMemAccessReady() {
		wf.MemAccess = make([]*mem.AccessReq, 0, 64)
		wf.WIMemRequested = make([]bool, 64)
		w.IncreasePC(wf, inst.ByteSize)
		w.Scheduler.Completed(wf)
	}

	return nil
}
