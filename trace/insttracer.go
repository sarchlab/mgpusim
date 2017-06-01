package trace

import (
	"log"

	"gitlab.com/yaotsu/core"
)

// A InstTracer is a LogHook that keep record of instruction execution status
type InstTracer struct {
	core.LogHookBase
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(logger *log.Logger) *InstTracer {
	t := new(InstTracer)
	t.Logger = logger
	return t
}
