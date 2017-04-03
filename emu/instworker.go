package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

// A InstWorker is where instructions got executed
type InstWorker interface {
	Run(wf *WfScheduleInfo, now core.VTimeInSec) error
}

// InstWorkerImpl is the standard implmentation of a InstWorker
type InstWorkerImpl struct {
	CU        gcn3.ComputeUnit
	Scheduler Scheduler
}

// Run will emulate the result of a instruction execution
func (w *InstWorkerImpl) Run(
	wf *WfScheduleInfo,
	now core.VTimeInSec,
) error {
	log.Printf("%f: Inst %s\n", now, wf.Inst.String())
	w.Scheduler.Completed(wf)
	return nil
}
