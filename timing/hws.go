package timing

import (
	"gitlab.com/yaotsu/core"
)

// An HWS is the hardware scheduling unit in AMD GCN3+ GPUs.
type HWS struct {
	*core.BasicComponent
}
