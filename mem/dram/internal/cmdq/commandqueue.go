// Package cmdq provides command queue implementations
package cmdq

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A CommandQueue is a queue of command that needs to be executed by a rank or
// a bank.
type CommandQueue interface {
	GetCommandToIssue(
		now sim.VTimeInSec,
	) *signal.Command
	CanAccept(command *signal.Command) bool
	Accept(command *signal.Command)
}
