package driver

import (
	"gitlab.com/akita/akita"
)

// CommandHookInfo carries the information provided to hooks that are
// triggered by Comands.
type CommandHookInfo struct {
	Now     akita.VTimeInSec
	IsStart bool
	Queue   *CommandQueue
}
