package smsp

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

type WarpStatus int

const SMSPSchedulerIssueSpeed = 999

const SMSPSchedulerIssuepolicy = "FCFS" // "ROUND_ROBIN" or "FCFS"

const (
	WarpStatusReady WarpStatus = iota
	WarpStatusWaiting
	WarpStatusRunning
)

type SMSPWarpUnit struct {
	ID                   uint64
	Trace                trace.WarpTrace
	Status               WarpStatus
	UnfinishedInstsCount uint64
	NextIssueInstIndex   uint64
	InFlightPipelines    []PipelineInstance
	Scoreboard           Scoreboard
}

type IssueDecision struct {
	WarpUnit *SMSPWarpUnit
	Pipeline *PipelineInstance
}

type SMSPSWarpScheduler struct {
	WarpUnits      []SMSPWarpUnit
	NextIssueIndex int
	NextWarpID     uint64
}

func NewSMSPScheduler() *SMSPSWarpScheduler {
	return &SMSPSWarpScheduler{
		WarpUnits:      []SMSPWarpUnit{},
		NextIssueIndex: 0,
	}
}

func isMemoryPipeStage(stageName string) bool {
	return stageName == "MemoryPipeRead" || stageName == "MemoryPipeWrite"
}

func regsToNames(inst *trace.InstructionTrace) (srcRegs []string, dstRegs []string) {
	srcRegs = make([]string, 0, len(inst.SrcRegs))
	dstRegs = make([]string, 0, len(inst.DestRegs))

	for _, r := range inst.SrcRegs {
		srcRegs = append(srcRegs, r.Name)
	}
	for _, r := range inst.DestRegs {
		dstRegs = append(dstRegs, r.Name)
	}

	return srcRegs, dstRegs
}

func (wu *SMSPWarpUnit) HasMoreToIssue() bool {
	return wu.NextIssueInstIndex < wu.Trace.InstructionsCount()
}

func (wu *SMSPWarpUnit) NextInstruction() *trace.InstructionTrace {
	if !wu.HasMoreToIssue() {
		return nil
	}
	return &wu.Trace.Instructions[wu.NextIssueInstIndex]
}

func (wu *SMSPWarpUnit) updateStatus() {
	switch {
	case wu.HasMoreToIssue():
		wu.Status = WarpStatusReady
	case len(wu.InFlightPipelines) > 0:
		wu.Status = WarpStatusRunning
	default:
		wu.Status = WarpStatusRunning
	}
}

func (s *SMSPSWarpScheduler) issueWarps(resourcePool *ResourcePool) []*IssueDecision {
	issued := []*IssueDecision{}
	startIndex := s.NextIssueIndex
	totalWarps := len(s.WarpUnits)
	checked := 0

	for len(issued) < SMSPSchedulerIssueSpeed && checked < totalWarps {
		idx := (startIndex + checked) % totalWarps
		wu := &s.WarpUnits[idx]

		if wu.UnfinishedInstsCount == 0 || !wu.HasMoreToIssue() {
			checked++
			continue
		}

		inst := wu.NextInstruction()
		pipe := NewPipelineInstance(inst, wu)
		stage := pipe.CurrentStage()
		if stage == nil {
			checked++
			continue
		}

		if wu.Scoreboard.HasConflict(pipe.SrcRegs, pipe.DstRegs) {
			checked++
			continue
		}

		if !resourcePool.Reserve(stage.Def.Unit) {
			checked++
			continue
		}

		wu.Scoreboard.MarkIssued(pipe.SrcRegs, pipe.DstRegs)
		wu.InFlightPipelines = append(wu.InFlightPipelines, *pipe)
		pipe = &wu.InFlightPipelines[len(wu.InFlightPipelines)-1]
		wu.NextIssueInstIndex++
		wu.updateStatus()

		issued = append(issued, &IssueDecision{
			WarpUnit: wu,
			Pipeline: pipe,
		})
		checked++
	}

	if totalWarps == 0 {
		return issued
	}

	if SMSPSchedulerIssuepolicy == "ROUND_ROBIN" {
		s.NextIssueIndex = (startIndex + checked) % totalWarps
	} else if SMSPSchedulerIssuepolicy == "FCFS" {
		s.NextIssueIndex = 0
	} else {
		panic("unsupported issue policy")
	}
	return issued
}

func (s *SMSPSWarpScheduler) insertWarp(warp trace.WarpTrace) bool {
	s.NextWarpID++
	newWarpUnit := SMSPWarpUnit{
		ID:                   s.NextWarpID,
		Trace:                warp,
		Status:               WarpStatusReady,
		UnfinishedInstsCount: warp.InstructionsCount(),
		NextIssueInstIndex:   0,
		InFlightPipelines:    []PipelineInstance{},
		Scoreboard:           *NewScoreboard(),
	}
	if len(warp.Instructions) == 0 {
		panic(fmt.Sprintf("warp (ID: %d) has no instructions", warp.ID))
	}

	s.WarpUnits = append(s.WarpUnits, newWarpUnit)
	return true
}

func (s *SMSPSWarpScheduler) insertWarps(warps []trace.WarpTrace) bool {
	for _, warp := range warps {
		s.insertWarp(warp)
	}
	return true
}

func (s *SMSPSWarpScheduler) isEmpty() bool {
	return len(s.WarpUnits) == 0
}

func (s *SMSPSWarpScheduler) warp(id uint64) *SMSPWarpUnit {
	for i := range s.WarpUnits {
		if s.WarpUnits[i].ID == id {
			return &s.WarpUnits[i]
		}
	}
	panic(fmt.Sprintf("unknown resident warp %d", id))
}
func (s *SMSPSWarpScheduler) pipeline(ref PipelineRef) *PipelineInstance {
	w := s.warp(ref.WarpID)
	for i := range w.InFlightPipelines {
		if w.InFlightPipelines[i].InstIndex == ref.InstIndex {
			return &w.InFlightPipelines[i]
		}
	}
	panic(fmt.Sprintf("unknown pipeline %v", ref))
}
