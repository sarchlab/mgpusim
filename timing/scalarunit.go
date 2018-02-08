package timing

import "gitlab.com/yaotsu/core"

// A ScalarUnit performs Scalar operations
type ScalarUnit struct {
	cu *ComputeUnit

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// NewScalarUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewScalarUnit(cu *ComputeUnit) *ScalarUnit {
	u := new(ScalarUnit)
	u.cu = cu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *ScalarUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *ScalarUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
}

// Run executes three pipeline stages that are controlled by the ScalarUnit
func (u *ScalarUnit) Run(now core.VTimeInSec) {
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *ScalarUnit) runReadStage(now core.VTimeInSec) {
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

func (u *ScalarUnit) runExecStage(now core.VTimeInSec) {
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

func (u *ScalarUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}