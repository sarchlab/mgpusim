package cu

import (
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	alu emu.ALU
}

// NewBranchUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewBranchUnit(
	cu *ComputeUnit,
	alu emu.ALU,
) *BranchUnit {
	u := new(BranchUnit)
	u.cu = cu
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.state().Read == 0
}

// IsIdle checks idleness
func (u *BranchUnit) IsIdle() bool {
	u.state().Idle = (u.state().Read == 0) && (u.state().Write == 0) && (u.state().Exec == 0)
	return u.state().Idle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(
	wave *Wavefront,
) {
	u.state().Read = u.cu.useWave(wave)
}

// Run executes three pipeline stages that are controlled by the BranchUnit
func (u *BranchUnit) Run() bool {
	madeProgress := false
	madeProgress = u.runWriteStage() || madeProgress
	madeProgress = u.runExecStage() || madeProgress
	madeProgress = u.runReadStage() || madeProgress
	return madeProgress
}

func (u *BranchUnit) runReadStage() bool {
	if u.state().Read == 0 {
		return false
	}

	if u.state().Exec == 0 {
		u.state().Exec = u.state().Read
		u.state().Read = 0

		return true
	}
	return false
}

func (u *BranchUnit) runExecStage() bool {
	if u.state().Exec == 0 {
		return false
	}

	if u.state().Write == 0 {
		u.alu.Run(u.cu.wave(u.state().Exec).emulation())

		u.state().Write = u.state().Exec
		u.state().Exec = 0
		return true
	}
	return false
}

func (u *BranchUnit) runWriteStage() bool {
	if u.state().Write == 0 {
		return false
	}

	wave := u.cu.wave(u.state().Write)
	u.cu.completeInstruction(wave, wave.InstID())

	wave.execution().InstBuffer = nil
	if wave.Inst().AssignsPC() {
		// The PC already holds the address to continue at. A PC-relative
		// branch leaves it one instruction short of that address.
		wave.execution().State = wavefront.WfReady
	} else {
		u.cu.UpdatePCAndSetReady(wave)
	}
	wave.execution().InstBufferStartPC = wave.PC() & 0xffffffffffffffc0
	u.state().Write = 0
	u.state().Idle = false
	return true
}

func (u *BranchUnit) state() *StageState { return &u.cu.comp.State.Branch }
