package timing

import (
	"gitlab.com/yaotsu/core"
)

// An ACE is short for Asynchronous Compute Engine in AMD GPUs, it dispatches
// workgroups/wavefronts to compute units.
type ACE struct {
	*core.BasicComponent
}
