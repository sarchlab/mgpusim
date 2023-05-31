package org

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A Bank is a DRAM Bank. It contains a number of rows and columns.
type Bank interface {
	tracing.NamedHookable

	GetReadyCommand(
		now sim.VTimeInSec,
		cmd *signal.Command,
	) *signal.Command
	StartCommand(now sim.VTimeInSec, cmd *signal.Command)
	UpdateTiming(cmdKind signal.CommandKind, cycleNeeded int)
	Tick(now sim.VTimeInSec) bool
}
