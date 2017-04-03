package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A InstWorker is where instructions got executed
type InstWorker interface {
	Run(wf *Wavefront, inst *disasm.Instruction, now core.VTimeInSec) error
}

// InstWorkerImpl is the standard implmentation of a InstWorker
type InstWorkerImpl struct {
	CU        gcn3.ComputeUnit
	Scheduler Scheduler
}

// Run will emulate the result of a instruction execution
func (w *InstWorkerImpl) Run(
	wf *Wavefront,
	inst *disasm.Instruction,
	now core.VTimeInSec,
) error {
	log.Printf("Inst: %s\n", inst.String())
	return nil
}
