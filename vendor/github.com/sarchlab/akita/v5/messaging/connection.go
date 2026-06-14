package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
)

// A Connection is responsible for delivering messages to its destination.
type Connection interface {
	naming.Named
	hooking.Hookable

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(port Port)

	NotifySend()
}
