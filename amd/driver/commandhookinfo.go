package driver

import (
	"github.com/sarchlab/akita/v5/timing"
)

// CommandHookInfo carries the information provided to hooks that are
// triggered by Comands.
type CommandHookInfo struct {
	Now     timing.VTimeInPicoSec
	IsStart bool
	Queue   *CommandQueue
}
