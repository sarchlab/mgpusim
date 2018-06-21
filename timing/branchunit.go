package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
)

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// NewBranchUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewBranchUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	alu emu.ALU,
) *BranchUnit {
	u := new(BranchUnit)
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, wave.inst, "ReadStart"})
}

// Run executes three pipeline stages that are controlled by the BranchUnit
func (u *BranchUnit) Run(now core.VTimeInSec) {
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *BranchUnit) runReadStage(now core.VTimeInSec) {
	if u.toRead == nil {
		return
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *BranchUnit) runExecStage(now core.VTimeInSec) {
	if u.toExec == nil {
		return
	}

	if u.toWrite == nil {
		u.alu.Run(u.toExec)
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "ExecEnd"})
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "WriteStart"})

		u.toWrite = u.toExec
		u.toExec = nil
	}
}

func (u *BranchUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}
