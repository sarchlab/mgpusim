package smsp

import (
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type Stage struct {
	Def  StageDef
	Left int
}

// Running pipeline instance
type PipelineInstance struct {
	Warp   *SMSPWarpUnit
	Stages []Stage
	PC     int // current stage index
	Done   bool
}

func NewPipelineInstance(inst *trace.InstructionTrace, warp *SMSPWarpUnit) *PipelineInstance {
	tpl, ok := PipelineTable[inst.OpCode.String()]
	if !ok {
		tpl = defaultStages(inst.OpCode.String())
	}

	stages := make([]Stage, len(tpl.Stages))
	for i, s := range tpl.Stages {
		stages[i] = Stage{Def: s, Left: s.Latency}
	}
	return &PipelineInstance{Warp: warp, Stages: stages, PC: 0, Done: false}
}

// Progress pipeline by one cycle
func (p *PipelineInstance) Tick(rsrc *ResourcePool) bool {
	if p.Done {
		return true
	}

	// guard against empty stage list
	if p.PC < 0 || p.PC >= len(p.Stages) {
		p.Done = true
		return true
	}

	stage := &p.Stages[p.PC]

	// Try reserve on first cycle of stage
	if stage.Left == stage.Def.Latency && stage.Def.Unit != UnitNone {
		if !rsrc.Reserve(stage.Def.Unit, stage.Def.UnitsUsed) {
			return false // stall due to resource conflict
		}
	}

	stage.Left--

	// Stage finished
	if stage.Left == 0 {
		// release resources
		if stage.Def.Unit != UnitNone {
			rsrc.Release(stage.Def.Unit, stage.Def.UnitsUsed)
		}

		p.PC++
		if p.PC == len(p.Stages) {
			p.Done = true
			return true
		}
	}
	return false
}
