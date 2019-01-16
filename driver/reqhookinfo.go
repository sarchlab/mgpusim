package driver

import (
	"gitlab.com/akita/akita"
)

// ReqHookInfo is the information that the driver send to the request hooks
type ReqHookInfo struct {
	CommandID string
	EventType string
	Now       akita.VTimeInSec
}
