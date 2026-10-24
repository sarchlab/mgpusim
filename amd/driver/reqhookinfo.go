package driver

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ReqHookInfo is the information that the driver send to the request hooks
type ReqHookInfo struct {
	CommandID uint64
	EventType string
	Now       timing.VTimeInPicoSec
}
