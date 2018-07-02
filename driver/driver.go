package driver

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
)

// Driver is an Yaotsu component that controls the simulated GPUs
type Driver struct {
	*core.ComponentBase

	engine core.Engine

	memoryMasks map[*mem.Storage]*MemoryMask

	ToGPUs *core.Port
}

// NewDriver creates a new driver
func NewDriver(engine core.Engine) *Driver {
	driver := new(Driver)
	driver.ComponentBase = core.NewComponentBase("driver")

	driver.engine = engine
	driver.memoryMasks = make(map[*mem.Storage]*MemoryMask)

	driver.ToGPUs = core.NewPort(driver)

	return driver
}

func (d *Driver) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	// Do nothing
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e core.Event) error {
	switch e := e.(type) {
	case *LaunchKernelEvent:
		return d.HandleLaunchKernelEvent(e)

	default:
		log.Panicf("Unable to process event")
	}
	return nil
}
