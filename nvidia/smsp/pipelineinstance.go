package smsp

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type Stage struct {
	Def  StageDef
	Left int
}

// One PipelineInstance represents one issued instruction.
type PipelineInstance struct {
	Warp                  *SMSPWarpUnit
	Inst                  *trace.InstructionTrace
	Stages                []Stage
	InstructionOpcode     string
	SrcRegs               []string
	DstRegs               []string
	PC                    int // current active stage index
	OperandReadStageIndex int
	Done                  bool
	SrcRegsReleased       bool
	DstRegsReleased       bool
}

func NewPipelineInstance(inst *trace.InstructionTrace, warp *SMSPWarpUnit) *PipelineInstance {
	tpl := getPipelineStages(inst.OpCode.String())

	stages := make([]Stage, len(tpl.Stages))
	for i, s := range tpl.Stages {
		stages[i] = Stage{Def: s, Left: s.Cycles}
	}

	srcRegs, dstRegs := regsToNames(inst)
	p := &PipelineInstance{
		Warp:              warp,
		Inst:              inst,
		Stages:            stages,
		InstructionOpcode: tpl.Opcode,
		SrcRegs:           srcRegs,
		DstRegs:           dstRegs,
		PC:                0,
		Done:              false,
	}
	p.advanceToNextActiveStage()
	p.OperandReadStageIndex = p.PC
	return p
}

func (p *PipelineInstance) advanceToNextActiveStage() {
	for p.PC < len(p.Stages) && p.Stages[p.PC].Left == 0 {
		p.PC++
	}
	if p.PC >= len(p.Stages) {
		p.Done = true
	}
}

func (p *PipelineInstance) CurrentStage() *Stage {
	if p.Done || p.PC < 0 || p.PC >= len(p.Stages) {
		return nil
	}
	return &p.Stages[p.PC]
}

func (p *PipelineInstance) IsMemoryPipeline() bool {
	stage := p.CurrentStage()
	return stage != nil && isMemoryPipeStage(stage.Def.Name)
}

func (p *PipelineInstance) ReadyToReleaseSrcRegs() bool {
	if p.SrcRegsReleased {
		return false
	}
	if p.Done {
		return true
	}
	if p.IsMemoryPipeline() {
		return false
	}
	return p.PC != p.OperandReadStageIndex
}

func (p *PipelineInstance) MarkSrcRegsReleased() {
	p.SrcRegsReleased = true
}

func (p *PipelineInstance) MarkDstRegsReleased() {
	p.DstRegsReleased = true
}

func (p *PipelineInstance) MarkMemoryRequestSent() {
	stage := p.CurrentStage()
	if stage == nil || !isMemoryPipeStage(stage.Def.Name) {
		log.Panic("MarkMemoryRequestSent called for non-memory pipeline")
	}
	if stage.Left != 2 {
		log.Panic("memory pipeline should have Left == 2 before request is sent")
	}
	stage.Left = 1
}

func (p *PipelineInstance) MarkMemoryResponseReady() {
	stage := p.CurrentStage()
	if stage == nil || !isMemoryPipeStage(stage.Def.Name) {
		log.Panic("MarkMemoryResponseReady called for non-memory pipeline")
	}
	if stage.Left == 0 {
		return
	}
	if stage.Left != 1 {
		log.Panic("memory pipeline should have Left == 1 when response returns")
	}
	stage.Left = 0
	p.advanceToNextActiveStage()
}

// Memory instructions are progressed by explicit request/response handling.
func (p *PipelineInstance) Tick() bool {
	if p.Done {
		return true
	}

	stage := p.CurrentStage()
	if stage == nil {
		p.Done = true
		return true
	}

	if isMemoryPipeStage(stage.Def.Name) {
		return false
	}

	stage.Left--
	if stage.Left < 0 {
		log.Panic("pipeline stage left cycles went negative")
	}

	if stage.Left == 0 {
		p.PC++
		p.advanceToNextActiveStage()
	}

	return true
}
