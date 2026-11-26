package cu

import (
	"strings"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// twoVGPRNonF64Insts lists the VALU instructions that move or compute a pair
// of VGPRs per lane although their name does not say f64: the packed-f32
// family, which runs two f32 operations per lane, and the 64-bit move.
var twoVGPRNonF64Insts = map[string]bool{
	"v_pk_fma_f32":  true,
	"v_pk_mul_f32":  true,
	"v_pk_add_f32":  true,
	"v_pk_mov_b32":  true,
	"v_mov_b64_e32": true,
}

// usesTwoVGPRsPerLane reports whether a VALU instruction is timed on the
// double-width datapath: half as many lanes per cycle in the SIMD unit, and
// twice the scoreboard latency. It is the single owner of that classification
// for the SIMD unit and the scoreboard.
//
// f64 instructions are recognized by name. The instructions in
// twoVGPRNonF64Insts are added because timing them as 32-bit operations made
// the MI300X fp32_throughput loop, two v_pk_fma_f32 and one v_mov_b64 per
// iteration, run at 4.40 ns per FMA against 7.73 ns measured on hardware.
//
// The list is interim. It does not use the decoder's operand widths, which
// are not consistent enough for timing, and it leaves 64-bit integer
// instructions on the 32-bit path because no measurement says otherwise.
// Measured per-instruction costs should replace it.
func usesTwoVGPRsPerLane(instName string) bool {
	return strings.Contains(instName, "f64") || twoVGPRNonF64Insts[instName]
}

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	hooking.HookableBase

	cu    *ComputeUnit
	index int

	name string

	alu emu.ALU

	// Pipeline mode fields (used when scoreboardEnabled)
	pipelineCapacity int

	scoreboardEnabled bool

	NumSinglePrecisionUnit int
}

// NewSIMDUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewSIMDUnit(
	cu *ComputeUnit,
	name string,
	alu emu.ALU,
) *SIMDUnit {
	u := new(SIMDUnit)
	u.name = name
	u.cu = cu
	u.alu = alu

	u.NumSinglePrecisionUnit = 16

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) CanAcceptWave() bool {
	if u.scoreboardEnabled {
		return len(u.state().Slots) < u.pipelineCapacity
	}
	return u.state().Exec == 0
}

// IsIdle checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) IsIdle() bool {
	if u.scoreboardEnabled {
		u.state().Idle = len(u.state().Slots) == 0
		return u.state().Idle
	}
	u.state().Idle = (u.state().Exec == 0)
	return u.state().Idle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *Wavefront) {
	cycleLeft := 64 / u.NumSinglePrecisionUnit
	if usesTwoVGPRsPerLane(wave.Inst().InstName) {
		cycleLeft = 64 / (u.NumSinglePrecisionUnit / 2)
	}

	if u.scoreboardEnabled {
		slot := SIMDSlotState{
			WF:     u.cu.useWave(wave),
			Cycles: cycleLeft,
		}
		u.state().Slots = append(u.state().Slots, slot)
		u.state().Slots[len(u.state().Slots)-1].Task = u.logPipelineTaskStart(wave.InstID())
		return
	}

	u.state().Exec = u.cu.useWave(wave)
	u.state().Cycles = cycleLeft
	u.state().Task = u.logPipelineTaskStart(u.cu.wave(u.state().Exec).InstID())
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run() bool {
	if u.scoreboardEnabled {
		return u.runPipelined()
	}
	return u.runExecStage()
}

func (u *SIMDUnit) runPipelined() bool {
	if len(u.state().Slots) == 0 {
		return false
	}

	madeProgress := false
	remaining := make([]SIMDSlotState, 0, len(u.state().Slots))

	for _, slot := range u.state().Slots {
		slot.Cycles--
		madeProgress = true

		if slot.Cycles <= 0 {
			u.alu.Run(u.cu.wave(slot.WF).emulation())
			u.cu.UpdatePCAndSetReady(u.cu.wave(slot.WF))

			u.logPipelineTaskEnd(slot.Task)
			u.cu.completeInstruction(u.cu.wave(slot.WF), u.cu.wave(slot.WF).InstID())
		} else {
			remaining = append(remaining, slot)
		}
	}

	u.state().Slots = remaining
	return madeProgress
}

func (u *SIMDUnit) runExecStage() bool {
	if u.state().Exec == 0 {
		return false
	}

	u.state().Cycles--
	if u.state().Cycles > 0 {
		return true
	}

	u.alu.Run(u.cu.wave(u.state().Exec).emulation())
	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Exec))

	u.logPipelineTaskEnd(u.state().Task)
	u.cu.completeInstruction(u.cu.wave(u.state().Exec), u.cu.wave(u.state().Exec).InstID())

	u.state().Exec = 0
	return true
}

// logPipelineTaskStart starts the per-SIMD pipeline task for the given
// instruction and returns the task ID (v4 used the string ID
// inst+"_simd_exec").
//
// The task keeps the SIMD's owner name and What label for trace identity.
// SIMD views are not registered simulation entities. The sample reporter uses
// one simulation observer to route completed pipeline tasks by their SIMD name
// to per-SIMD BusyTimeTracers; it does not subscribe on the SIMD view itself.
// The parent instruction task belongs to the CU component.
func (u *SIMDUnit) logPipelineTaskStart(inst uint64) uint64 {
	taskID := u.cu.comp.TaskTracker().StartTask(u, tasks.StartInfo{
		ParentID: u.cu.comp.State.InstTasks[inst],
		Kind:     "pipeline",
		What:     u.Name() + "." + u.cu.execUnitToString(u.cu.instruction(inst).ExeUnit),
	})

	return taskID
}

func (u *SIMDUnit) logPipelineTaskEnd(taskID uint64) {
	u.cu.comp.TaskTracker().EndTask(taskID)
}

// Name names the unit
func (u *SIMDUnit) Name() string {
	return u.name
}

// CurrentTime returns the current time of the compute unit the SIMD unit
// belongs to. It makes the SIMD unit a tracing domain.
func (u *SIMDUnit) CurrentTime() timing.VTimeInPicoSec {
	return u.cu.comp.CurrentTime()
}

func (u *SIMDUnit) TaskTracker() *tasks.Tracker { return u.cu.comp.TaskTracker() }

func (u *SIMDUnit) state() *SIMDState { return &u.cu.comp.State.SIMDs[u.index] }
