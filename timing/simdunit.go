package timing

import "gitlab.com/yaotsu/core"

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	cu *ComputeUnit

	toRead        *Wavefront
	toExec        *Wavefront
	execCycleLeft int
	toWrite       *Wavefront
}

// NewSIMDUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewSIMDUnit(cu *ComputeUnit) *SIMDUnit {
	u := new(SIMDUnit)
	u.cu = cu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ReadStart"})
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run(now core.VTimeInSec) {
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *SIMDUnit) runReadStage(now core.VTimeInSec) {
	if u.toRead == nil {
		return
	}

	if u.toExec == nil {
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, "ExecStart"})

		u.toExec = u.toRead
		u.execCycleLeft = 4
		u.toRead = nil
	}
}

func (u *SIMDUnit) runExecStage(now core.VTimeInSec) {
	if u.toExec == nil {
		return
	}

	u.execCycleLeft--
	if u.execCycleLeft > 0 {
		return
	}

	if u.toWrite == nil {
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, "ExecEnd"})
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, "WriteStart"})

		u.toWrite = u.toExec
		u.toExec = nil
	}
}

func (u *SIMDUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}
