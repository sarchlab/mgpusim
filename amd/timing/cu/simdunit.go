package cu

import (
	"strings"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// simdPipelineSlot represents one in-flight wavefront in the SIMD pipeline.
type simdPipelineSlot struct {
	wf        *wavefront.Wavefront
	cycleLeft int
}

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	hooking.HookableBase

	cu *ComputeUnit

	name string

	alu emu.ALU

	toExec    *wavefront.Wavefront
	cycleLeft int

	// Pipeline mode fields (used when scoreboardEnabled)
	pipelineSlots    []*simdPipelineSlot
	pipelineCapacity int

	scoreboardEnabled bool

	NumSinglePrecisionUnit int

	isCDNA3 bool

	isIdle bool
}

// isTranscendental returns true if the instruction name represents a
// transcendental operation (v_rcp, v_rsq, v_sqrt, v_log, v_exp, v_sin,
// v_cos and their f16/f64 variants).
func isTranscendental(instName string) bool {
	prefixes := []string{
		"v_rcp_", "v_rsq_", "v_sqrt_",
		"v_log_", "v_exp_",
		"v_sin_", "v_cos_",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(instName, p) {
			return true
		}
	}
	return false
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
		return len(u.pipelineSlots) < u.pipelineCapacity
	}
	return u.toExec == nil
}

// IsIdle checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) IsIdle() bool {
	if u.scoreboardEnabled {
		u.isIdle = len(u.pipelineSlots) == 0
		return u.isIdle
	}
	u.isIdle = (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *wavefront.Wavefront) {
	instName := wave.Inst().InstName

	cycleLeft := 64 / u.NumSinglePrecisionUnit // FP32 default: 4 cycles

	if isTranscendental(instName) {
		cycleLeft = 16
	} else if strings.Contains(instName, "f64") {
		if u.isCDNA3 {
			cycleLeft = 64 / u.NumSinglePrecisionUnit // CDNA3: full-rate FP64
		} else {
			cycleLeft = 64 / (u.NumSinglePrecisionUnit / 2) // half-rate: 8 cycles
		}
	}

	if u.scoreboardEnabled {
		slot := &simdPipelineSlot{
			wf:        wave,
			cycleLeft: cycleLeft,
		}
		u.pipelineSlots = append(u.pipelineSlots, slot)
		u.logPipelineTask(wave.DynamicInst(), false)
		return
	}

	u.toExec = wave
	u.cycleLeft = cycleLeft
	u.logPipelineTask(u.toExec.DynamicInst(), false)
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run() bool {
	if u.scoreboardEnabled {
		return u.runPipelined()
	}
	return u.runExecStage()
}

func (u *SIMDUnit) runPipelined() bool {
	if len(u.pipelineSlots) == 0 {
		return false
	}

	madeProgress := false
	remaining := make([]*simdPipelineSlot, 0, len(u.pipelineSlots))

	for _, slot := range u.pipelineSlots {
		slot.cycleLeft--
		madeProgress = true

		if slot.cycleLeft <= 0 {
			u.alu.Run(slot.wf)
			u.cu.UpdatePCAndSetReady(slot.wf)

			u.logPipelineTask(slot.wf.DynamicInst(), true)
			u.cu.logInstTask(slot.wf, slot.wf.DynamicInst(), true)
		} else {
			remaining = append(remaining, slot)
		}
	}

	u.pipelineSlots = remaining
	return madeProgress
}

func (u *SIMDUnit) runExecStage() bool {
	if u.toExec == nil {
		return false
	}

	u.cycleLeft--
	if u.cycleLeft > 0 {
		return true
	}

	u.alu.Run(u.toExec)
	u.cu.UpdatePCAndSetReady(u.toExec)

	u.logPipelineTask(u.toExec.DynamicInst(), true)
	u.cu.logInstTask(u.toExec, u.toExec.DynamicInst(), true)

	u.toExec = nil
	return true
}

// Flush flushes
func (u *SIMDUnit) Flush() {
	u.toExec = nil
	u.pipelineSlots = u.pipelineSlots[:0]
}

func (u *SIMDUnit) logPipelineTask(
	inst *wavefront.Inst,
	completed bool,
) {
	if completed {
		tracing.EndTask(
			inst.PipelineTaskID,
			u,
		)
		return
	}

	inst.PipelineTaskID = sim.GetIDGenerator().Generate()
	tracing.StartTask(
		inst.PipelineTaskID,
		inst.ID,
		u,
		"pipeline",
		u.cu.execUnitToString(inst.ExeUnit),
		// inst.InstName,
		nil,
	)
}

// Name names the unit
func (u *SIMDUnit) Name() string {
	return u.name
}
