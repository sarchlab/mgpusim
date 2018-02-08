package timing

import "gitlab.com/yaotsu/core"

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// NewBranchUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewBranchUnit(cu *ComputeUnit) *BranchUnit {
	u := new(BranchUnit)
	u.cu = cu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ReadStart"})
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
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *BranchUnit) runExecStage(now core.VTimeInSec) {
	if u.toExec == nil {
		return
	}

	if u.toWrite == nil {
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, "ExecEnd"})
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, "WriteStart"})

		u.toWrite = u.toExec
		u.toExec = nil
	}
}

func (u *BranchUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}
