package sim

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
)

// A Component is an element being simulated in Akita.
//
// Component is the unifying interface for all simulation elements that
// own ports and can be notified of port activity. Event handling
// (Handler interface) is intentionally NOT part of Component —
// event dispatch is handled by the Engine via Event.HandlerID().
type Component interface {
	naming.Named
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}

// ComponentBase provides some functions that other component can use.
type ComponentBase struct {
	sync.Mutex
	hooking.HookableBase
	*PortOwnerBase

	name string
}

// NewComponentBase creates a new ComponentBase
func NewComponentBase(name string) *ComponentBase {
	naming.MustBeValid(name)

	c := new(ComponentBase)
	c.name = name
	c.PortOwnerBase = NewPortOwnerBase()

	return c
}

// Name returns the name of the BasicComponent
func (c *ComponentBase) Name() string {
	return c.name
}
