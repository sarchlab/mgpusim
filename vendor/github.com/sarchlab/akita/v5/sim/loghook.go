package sim

import (
	"log"
)

// LogHookBase provides the common logic for all LogHooks
type LogHookBase struct {
	*log.Logger
}
