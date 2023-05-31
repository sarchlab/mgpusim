package driver

import (
	"github.com/sarchlab/akita/v3/sim"
)

// CommandHookInfo carries the information provided to hooks that are
// triggered by Comands.
type CommandHookInfo struct {
	Now     sim.VTimeInSec
	IsStart bool
	Queue   *CommandQueue
}
