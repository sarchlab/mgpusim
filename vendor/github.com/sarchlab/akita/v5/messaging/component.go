package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
)

// A PortOwner is an element that can communicate with others through ports.
// A component declares the ports it has with DeclarePort and receives their
// instances externally through AssignPort.
type PortOwner interface {
	DeclarePort(name string, roles ...*Role)
	AssignPort(name string, port Port)
	GetPortByName(name string) Port
	Ports() []Port
}

// A Component is an element that owns ports and can be notified of port
// activity.
type Component interface {
	naming.Named
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}
