package cu

import (
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// A LDSUnit performs Scalar operations
type LDSUnit struct {
	cu *ComputeUnit

	alu emu.ALU
}

// NewLDSUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewLDSUnit(
	cu *ComputeUnit,
	alu emu.ALU,
) *LDSUnit {
	u := new(LDSUnit)
	u.cu = cu
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *LDSUnit) CanAcceptWave() bool {
	return u.state().Read == 0
}

// IsIdle checks idleness
func (u *LDSUnit) IsIdle() bool {
	u.state().Idle = (u.state().Read == 0) && (u.state().Write == 0) && (u.state().Exec == 0)
	return u.state().Idle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *LDSUnit) AcceptWave(wave *Wavefront) {
	u.state().Read = u.cu.useWave(wave)
}

// Run executes three pipeline stages that are controlled by the LDSUnit
func (u *LDSUnit) Run() bool {
	madeProgress := false
	madeProgress = u.runWriteStage() || madeProgress
	madeProgress = u.runExecStage() || madeProgress
	madeProgress = u.runReadStage() || madeProgress
	return madeProgress
}

func (u *LDSUnit) runReadStage() bool {
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

func (u *LDSUnit) runExecStage() bool {
	if u.state().Exec == 0 {
		return false
	}

	if u.state().Write != 0 {
		return false
	}

	if u.state().Cycles == 0 {
		u.alu.SetLDS(u.cu.wave(u.state().Exec).group().state().LDS)
		defer u.alu.SetLDS(nil)
		u.alu.Run(u.cu.wave(u.state().Exec).emulation())
		u.state().Cycles = 14
		return true
	}

	u.state().Cycles--
	if u.state().Cycles == 0 {
		u.state().Write = u.state().Exec
		u.state().Exec = 0
	}
	return true
}

func (u *LDSUnit) runWriteStage() bool {
	if u.state().Write == 0 {
		return false
	}

	u.cu.completeInstruction(u.cu.wave(u.state().Write), u.cu.wave(u.state().Write).DynamicInst())

	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Write))

	u.state().Write = 0
	return true
}

func (u *LDSUnit) state() *StageState { return &u.cu.comp.State.LDS }
