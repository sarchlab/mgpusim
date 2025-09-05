package smsp

import (
	// "fmt"

	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type WarpStatus int

const (
	WarpStatusDefault WarpStatus = iota
	WarpStatusWait
	WarpStatusStallNoInstruction
	WarpStatusStallAllocationStall
	WarpStatusSelected
	WarpStatusNotSelected
)

type SMSPWarpUnit struct {
	/*
		"Wait" means "waiting on a fixed-latency execution dependency";
		"Stall No Instruction" means "warp hasn't fetched an instruction yet";
		"Stall Allocation Stall" means "the scheduler can't allocate the warp yet: e.g., pending memory ops must retire"
		"Selected" means "the warp issued an instruction this cycle";
		"Not Selected" means "warp was eligible, but another warp was chosen to issue this cycle";
	*/
	warp   *trace.WarpTrace
	status WarpStatus
}

type SMSPSWarpScheduler struct {
	warpUnitList []*SMSPWarpUnit
}

func (s *SMSPSWarpScheduler) getFirstNotSelectedWarp() *SMSPWarpUnit {
	for _, warpUnit := range s.warpUnitList {
		if warpUnit.status == WarpStatusNotSelected {
			warpUnit.status = WarpStatusSelected
			return warpUnit
		}
	}
	return nil
}

func (s *SMSPSWarpScheduler) insertWarp(warp *trace.WarpTrace) *SMSPWarpUnit {
	newWarpUnit := &SMSPWarpUnit{
		warp:   warp,
		status: WarpStatusNotSelected,
	}
	s.warpUnitList = append(s.warpUnitList, newWarpUnit)
	return newWarpUnit
}
