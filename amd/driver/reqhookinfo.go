package driver

import (
	"github.com/sarchlab/akita/v5/sim"
)

// ReqHookInfo is the information that the driver send to the request hooks
type ReqHookInfo struct {
	CommandID string
	EventType string
	Now       sim.VTimeInSec
}
