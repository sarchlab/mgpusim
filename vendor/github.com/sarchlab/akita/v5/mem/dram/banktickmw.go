package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
)

type bankTickMW struct {
	comp      *modeling.Component[Spec, State, Resources]
	timing    dramTiming
	cmdCycles map[commandKind]int
}

// Tick runs tickBanks, issue, and tickSubTransQueue. Paused DRAM
// freezes the timing pipeline so in-flight transactions stay where
// they are; draining DRAM continues so the drain can converge.
func (m *bankTickMW) Tick() bool {
	next := &m.comp.State
	if next.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	spec := m.comp.Spec()
	next.TickCount++
	next.TotalCycles++

	progress := tickBanks(&spec, m.cmdCycles, next)

	// Handle periodic refresh
	refreshActive := m.handleRefresh(&spec, next)
	progress = refreshActive || progress

	// Only issue new commands if refresh is not in progress
	if !next.RefreshInProgress {
		progress = m.issue(&spec, next) || progress
	}

	progress = tickSubTransQueue(&spec, next) || progress

	return progress
}

// handleRefresh implements periodic refresh scheduling.
// It stalls command issuance for tRFC cycles every tREFI interval.
func (m *bankTickMW) handleRefresh(spec *Spec, next *State) bool {
	if spec.TREFI <= 0 {
		return false
	}

	// If refresh is in progress, count down
	if next.RefreshInProgress {
		next.RefreshCyclesRemaining--
		if next.RefreshCyclesRemaining <= 0 {
			next.RefreshInProgress = false
		}
		return true
	}

	// Countdown to next refresh
	next.RefreshCycleCounter++
	if next.RefreshCycleCounter >= spec.TREFI {
		next.RefreshInProgress = true
		next.RefreshCyclesRemaining = spec.TRFC
		next.RefreshCycleCounter = 0
		return true
	}

	return false
}

func (m *bankTickMW) issue(spec *Spec, next *State) bool {
	cmd := getCommandToIssue(spec, next)
	if cmd == nil {
		return false
	}

	bs := findBankStateByLocation(&next.BankStates, cmd.Location)
	if bs == nil {
		return false
	}

	startCommand(m.cmdCycles, next, bs, cmd)
	updateTiming(m.timing, next, cmd)

	return true
}
