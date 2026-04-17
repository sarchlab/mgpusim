package smsp

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"

	log "github.com/sirupsen/logrus"
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
	warp                 *trace.WarpTrace
	status               WarpStatus
	unfinishedInstsCount uint64
	nextIssueInstIndex   uint64
	InFlightPipelines    []*PipelineInstance
	Scoreboard           *Scoreboard
}

type IssueDecision struct {
	WarpUnit *SMSPWarpUnit
	Pipeline *PipelineInstance
}

type SMSPSWarpScheduler struct {
	warpUnitList   []*SMSPWarpUnit
	nextIssueIndex int
}

func NewSMSPScheduler() *SMSPSWarpScheduler {
	return &SMSPSWarpScheduler{
		warpUnitList:   []*SMSPWarpUnit{},
		nextIssueIndex: 0,
	}
}

func isIssueStage(stageName string) bool {
	return stageName == "Issue"
}

func isMemoryPipeStage(stageName string) bool {
	return stageName == "MemoryPipeRead" || stageName == "MemoryPipeWrite"
}

func isMemoryPipeReadStage(stageName string) bool {
	return stageName == "MemoryPipeRead"
}

func isMemoryPipeWriteStage(stageName string) bool {
	return stageName == "MemoryPipeWrite"
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
	return wu.nextIssueInstIndex < wu.warp.InstructionsCount()
}

func (wu *SMSPWarpUnit) NextInstruction() *trace.InstructionTrace {
	if !wu.HasMoreToIssue() {
		return nil
	}
	return wu.warp.Instructions[wu.nextIssueInstIndex]
}

func (wu *SMSPWarpUnit) updateStatus() {
	switch {
	case wu.HasMoreToIssue():
		wu.status = WarpStatusReady
	case len(wu.InFlightPipelines) > 0:
		wu.status = WarpStatusRunning
	default:
		wu.status = WarpStatusRunning
	}
}

func (s *SMSPSWarpScheduler) logWarpUnitList(smspName string, engineCurrentTime sim.VTimeInSec) {
	fmt.Printf("%.10f, %s's Scheduler has %d Warps:", engineCurrentTime, smspName, len(s.warpUnitList))
	for i, wu := range s.warpUnitList {
		nextOpcode := "<none>"
		nextStage := "<done>"
		if wu.HasMoreToIssue() {
			inst := wu.NextInstruction()
			nextOpcode = inst.OpCode.String()
			pipe := NewPipelineInstance(inst, wu)
			if stage := pipe.CurrentStage(); stage != nil {
				nextStage = stage.Def.Name
			}
		}
		fmt.Printf(" [wu %d/%d (status: %v): next issue %d/%d '%s' @ '%s', inflight=%d] [scoreboard: read #: %d, write #: %d]",
			i+1,
			len(s.warpUnitList),
			wu.status,
			wu.nextIssueInstIndex+1,
			wu.warp.InstructionsCount(),
			nextOpcode,
			nextStage,
			len(wu.InFlightPipelines),
			wu.Scoreboard.getNumOfRegReadBusy(),
			wu.Scoreboard.getNumOfRegWriteBusy())
	}
	fmt.Println()
}

func (s *SMSPSWarpScheduler) issueWarps(resourcePool *ResourcePool) []*IssueDecision {
	issued := []*IssueDecision{}
	startIndex := s.nextIssueIndex
	totalWarps := len(s.warpUnitList)
	checked := 0

	for len(issued) < SMSPSchedulerIssueSpeed && checked < totalWarps {
		idx := (startIndex + checked) % totalWarps
		wu := s.warpUnitList[idx]

		if wu.unfinishedInstsCount == 0 || !wu.HasMoreToIssue() {
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
		wu.InFlightPipelines = append(wu.InFlightPipelines, pipe)
		wu.nextIssueInstIndex++
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
		s.nextIssueIndex = (startIndex + checked) % totalWarps
	} else if SMSPSchedulerIssuepolicy == "FCFS" {
		s.nextIssueIndex = 0
	} else {
		log.Panic("unsupported issue policy")
	}
	return issued
}

func (s *SMSPSWarpScheduler) insertWarp(warp *trace.WarpTrace) bool {
	newWarpUnit := &SMSPWarpUnit{
		warp:                 warp,
		status:               WarpStatusReady,
		unfinishedInstsCount: warp.InstructionsCount(),
		nextIssueInstIndex:   0,
		InFlightPipelines:    []*PipelineInstance{},
		Scoreboard:           NewScoreboard(),
	}
	if len(warp.Instructions) == 0 {
		log.Panic(fmt.Sprintf("warp (ID: %d) has no instructions", warp.ID))
	}

	s.warpUnitList = append(s.warpUnitList, newWarpUnit)
	return true
}

func (s *SMSPSWarpScheduler) insertWarps(warps []*trace.WarpTrace) bool {
	for _, warp := range warps {
		s.insertWarp(warp)
	}
	return true
}

func (s *SMSPSWarpScheduler) isEmpty() bool {
	return len(s.warpUnitList) == 0
}

func (s *SMSPSWarpScheduler) removeFinishedWarps(warpUnit *SMSPWarpUnit) {
	for i, unit := range s.warpUnitList {
		if unit == warpUnit {
			s.warpUnitList = append(s.warpUnitList[:i], s.warpUnitList[i+1:]...)
			return
		}
	}
	log.Panic("warp unit is not implemented yet")
}
