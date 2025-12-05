package smsp

import (
	// "fmt"

	"fmt"
	"log"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type WarpStatus int

const SMSPSchedulerIssueSpeed = 999

const SMSPSchedulerIssuepolicy = "FCFS" // "ROUND_ROBIN" or "FCFS"

const (
	WarpStatusReady WarpStatus = iota
	WarpStatusWaiting
	WarpStatusRunning
	// WarpStatusDefault WarpStatus = iota
	// WarpStatusWait
	// WarpStatusStallNoInstruction
	// WarpStatusStallAllocationStall
	// WarpStatusSelected
	// WarpStatusNotSelected
)

type SMSPWarpUnit struct {
	/*
		"Wait" means "waiting on a fixed-latency execution dependency";
		"Stall No Instruction" means "warp hasn't fetched an instruction yet";
		"Stall Allocation Stall" means "the scheduler can't allocate the warp yet: e.g., pending memory ops must retire"
		"Selected" means "the warp issued an instruction this cycle";
		"Not Selected" means "warp was eligible, but another warp was chosen to issue this cycle";
	*/
	warp                 *trace.WarpTrace
	status               WarpStatus
	unfinishedInstsCount uint64
	Pipeline             *PipelineInstance
	// currentInstructionRemainingCycles uint64

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

// func (s *SMSPSWarpScheduler) issueWarp(startIndex int) (warpUnitIndex int, warpUnit *SMSPWarpUnit) {
// 	if startIndex >= len(s.warpUnitList) {
// 		return -1, nil
// 	}
// 	for i := startIndex; i < len(s.warpUnitList); i++ {
// 		warpUnit := s.warpUnitList[i]
// 		if warpUnit.status == WarpStatusReady { // || warpUnit.status == WarpStatusRunning
// 			warpUnit.status = WarpStatusRunning
// 			return i, warpUnit
// 		}
// 	}
// 	return -1, nil
// }

func isExecuteIssueOrMemoryPipeStage(stageName string) bool {
	// fmt.Printf("Checking if stage %s is an execute stage\n", stageName)
	return stageName == "Issue" || stageName == "MemoryPipe"
}

func (s *SMSPSWarpScheduler) logWarpUnitList(smspName string, engineCurrentTime sim.VTimeInSec) {
	fmt.Printf("%.10f, %s's Scheduler has %d Warps:", engineCurrentTime, smspName, len(s.warpUnitList))
	for i, wu := range s.warpUnitList {
		fmt.Printf(" [wu %d/%d: inst %d/%d '%s' @ '%s' stage (%d/%d)]",
			i+1,
			len(s.warpUnitList),
			wu.warp.InstructionsCount()-wu.unfinishedInstsCount+1,
			wu.warp.InstructionsCount(),
			wu.Pipeline.InstructionOpcode,
			wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name,
			wu.Pipeline.Stages[wu.Pipeline.PC].Def.Cycles-wu.Pipeline.Stages[wu.Pipeline.PC].Left+1,
			wu.Pipeline.Stages[wu.Pipeline.PC].Def.Cycles)
	}
	fmt.Println()
}

func (s *SMSPSWarpScheduler) issueWarps(resourcePool *ResourcePool) []*SMSPWarpUnit { // debugName string

	issued := []*SMSPWarpUnit{}
	startIndex := s.nextIssueIndex
	totalWarps := len(s.warpUnitList)
	checked := 0

	for len(issued) < SMSPSchedulerIssueSpeed && checked < totalWarps {
		idx := (startIndex + checked) % totalWarps
		wu := s.warpUnitList[idx]

		if (wu.status == WarpStatusReady || wu.status == WarpStatusRunning) && wu.unfinishedInstsCount > 0 {
			// instIdx := wu.warp.InstructionsCount() - wu.unfinishedInstsCount
			stageName := wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name
			// if strings.Contains(debugName, "GPU[0].SM[0].SMSP[0]") {
			// 	fmt.Printf("debug: stageName = %s\n", stageName)
			// }
			if isExecuteIssueOrMemoryPipeStage(stageName) {
				// if strings.Contains(debugName, "GPU[0].SM[0].SMSP[0]") {
				// 	fmt.Printf("debug: isExecuteIssueOrMemoryPipeStage")
				// }
				unitType := wu.Pipeline.Stages[wu.Pipeline.PC].Def.Unit
				if !resourcePool.Reserve(unitType) {
					checked++
					// if strings.Contains(debugName, "GPU[0].SM[0].SMSP[0]") {
					// 	fmt.Printf("debug: conflict")
					// }
					continue // resource conflict → skip
				}
			}
			// if strings.Contains(debugName, "GPU[0].SM[0].SMSP[0]") {
			// 	fmt.Printf("Issuing warp %d of SM's Scheduler: inst %d/%d '%s' @ '%s' stage (%d/%d)\n",
			// 		wu.warp.ID,
			// 		wu.warp.InstructionsCount()-wu.unfinishedInstsCount+1,
			// 		wu.warp.InstructionsCount(),
			// 		wu.Pipeline.InstructionOpcode,
			// 		wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name,
			// 		wu.Pipeline.Stages[wu.Pipeline.PC].Def.Cycles-wu.Pipeline.Stages[wu.Pipeline.PC].Left+1,
			// 		wu.Pipeline.Stages[wu.Pipeline.PC].Def.Cycles)
			// }
			wu.status = WarpStatusRunning
			issued = append(issued, wu)
		}

		checked++
	}
	if SMSPSchedulerIssuepolicy == "ROUND_ROBIN" {
		s.nextIssueIndex = (startIndex + checked) % totalWarps
	} else if SMSPSchedulerIssuepolicy == "FCFS" {
		s.nextIssueIndex = 0
	} else {
		log.Panic("unsupported issue policy")
	}

	return issued

	// issuedWarps := []*SMSPWarpUnit{}
	// startIndex := 0
	// for i := 0; i < SMSPSchedulerIssueSpeed; i++ {
	// 	var warpUnit *SMSPWarpUnit
	// 	startIndex, warpUnit = s.issueWarp(startIndex)
	// 	if warpUnit != nil {
	// 		issuedWarps = append(issuedWarps, warpUnit)
	// 		startIndex++ // Move to the next warp
	// 	} else {
	// 		break
	// 	}
	// }

	// return issuedWarps
}

func (s *SMSPSWarpScheduler) insertWarp(warp *trace.WarpTrace) bool {
	newWarpUnit := &SMSPWarpUnit{
		warp:                 warp,
		status:               WarpStatusReady,
		unfinishedInstsCount: warp.InstructionsCount(),
		Pipeline:             nil,
	}
	if len(warp.Instructions) == 0 {
		log.Panic("warp has no instructions")
	}
	inst := warp.Instructions[0]

	newWarpUnit.Pipeline = NewPipelineInstance(inst, newWarpUnit)

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
