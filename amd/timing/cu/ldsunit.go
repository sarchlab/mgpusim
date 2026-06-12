package cu

import (
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// A LDSUnit performs Scalar operations
type LDSUnit struct {
	cu *ComputeUnit

	alu      emu.ALU
	numBanks int

	toRead    *wavefront.Wavefront
	toExec    *wavefront.Wavefront
	toWrite   *wavefront.Wavefront
	cycleLeft int

	isIdle bool

	// Reusable scratch arrays to avoid per-call allocation.
	bankCounts [64]int
}

// NewLDSUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewLDSUnit(
	cu *ComputeUnit,
	alu emu.ALU,
	numBanks int,
) *LDSUnit {
	u := new(LDSUnit)
	u.cu = cu
	u.alu = alu
	u.numBanks = numBanks
	if u.numBanks <= 0 {
		u.numBanks = 32
	}
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *LDSUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// IsIdle checks idleness
func (u *LDSUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toWrite == nil) && (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *LDSUnit) AcceptWave(wave *wavefront.Wavefront) {
	u.toRead = wave
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
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.toExec = u.toRead
		u.toRead = nil
		return true
	}
	return false
}

func (u *LDSUnit) runExecStage() bool {
	if u.toExec == nil {
		return false
	}

	if u.toWrite != nil {
		return false
	}

	if u.cycleLeft == 0 {
		conflicts := u.computeBankConflicts(u.toExec)
		u.alu.SetLDS(u.toExec.WG.LDS)
		u.alu.Run(u.toExec)
		latency := 4
		if conflicts > latency {
			latency = conflicts
		}
		u.cycleLeft = latency
		return true
	}

	u.cycleLeft--
	if u.cycleLeft == 0 {
		u.toWrite = u.toExec
		u.toExec = nil
	}
	return true
}

// computeBankConflicts determines the number of bank conflicts for an LDS
// instruction by examining per-lane addresses against the configured number
// of banks. Returns 0 when address information is unavailable.
//
// The implementation uses a fixed-size array (bankCounts field) instead of
// allocating a new slice on every call, and applies a broadcast optimization:
// when multiple lanes read the identical address, real AMD LDS hardware
// broadcasts the result in a single cycle, so duplicate addresses are
// counted only once per bank.
func (u *LDSUnit) computeBankConflicts(wf *wavefront.Wavefront) int {
	inst := wf.Inst()
	if inst == nil || inst.Format == nil || inst.Format.FormatType != insts.DS {
		return 0
	}
	if inst.Addr == nil {
		return 0
	}

	// Guard against nil RegAccessor (e.g. in unit tests)
	if wf.RegAccessor == nil {
		return 0
	}

	exec := wf.EXEC()

	// Collect effective addresses for active lanes.
	var addrs [64]uint32
	active := 0
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		addr := uint32(wf.ReadOperand(inst.Addr, i))
		addr += inst.Offset0
		addrs[active] = addr
		active++
	}

	if active == 0 {
		return 0
	}

	// Broadcast optimization: deduplicate identical addresses.
	// When multiple lanes access the same address, the LDS can serve
	// them all in a single bank cycle via broadcast.
	var unique [64]uint32
	numUnique := 0
	for i := 0; i < active; i++ {
		dup := false
		for j := 0; j < numUnique; j++ {
			if addrs[i] == unique[j] {
				dup = true
				break
			}
		}
		if !dup {
			unique[numUnique] = addrs[i]
			numUnique++
		}
	}

	// Clear only the banks we'll touch (reuse fixed array on struct).
	for i := 0; i < u.numBanks; i++ {
		u.bankCounts[i] = 0
	}

	maxConflicts := 0
	for i := 0; i < numUnique; i++ {
		bank := int((unique[i] / 4) % uint32(u.numBanks))
		u.bankCounts[bank]++
		if u.bankCounts[bank] > maxConflicts {
			maxConflicts = u.bankCounts[bank]
		}
	}

	return maxConflicts
}

func (u *LDSUnit) runWriteStage() bool {
	if u.toWrite == nil {
		return false
	}

	u.cu.logInstTask(u.toWrite, u.toWrite.DynamicInst(), true)

	u.cu.UpdatePCAndSetReady(u.toWrite)

	u.toWrite = nil
	return true
}

// Flush clears the unit
func (u *LDSUnit) Flush() {
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
	u.cycleLeft = 0
}
