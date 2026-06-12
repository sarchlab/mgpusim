package writeback

import (
	"github.com/sarchlab/akita/v5/modeling"
)

// controlMW runs the flusher (flush/invalidate from controlPort,
// controls cache state).
type controlMW struct {
	comp    *modeling.Component[Spec, State]
	flusher *flusher
}

// Tick runs the flusher.
func (m *controlMW) Tick() bool {
	return m.flusher.Tick()
}
