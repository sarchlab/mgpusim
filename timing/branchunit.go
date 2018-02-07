package timing

import "gitlab.com/yaotsu/core"

// A BranchUnit performs branch operations
type BranchUnit struct {
	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
}

// Run executes three pipeline stages that are controlled by the BranchUnit
func (u *BranchUnit) Run(now core.VTimeInSec) {
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *BranchUnit) runReadStage(now core.VTimeInSec) {
	if u.toExec == nil {
		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *BranchUnit) runExecStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		u.toWrite = u.toExec
		u.toExec = nil
	}
}

func (u *BranchUnit) runWriteStage(now core.VTimeInSec) {
	u.toWrite.State = WfReady
	u.toWrite = nil
}
