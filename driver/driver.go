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
}

// NewDriver creates a new driver
func NewDriver(engine core.Engine) *Driver {
	driver := new(Driver)
	driver.ComponentBase = core.NewComponentBase("driver")

	driver.engine = engine

	driver.ComponentBase.AddPort("ToGPUs")

	driver.memoryMasks = make(map[*mem.Storage]*MemoryMask)

	return driver
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e core.Event) error {
	switch e := e.(type) {
	case *LaunchKernelEvent:
		return HandleLaunchKernelEvent(e)

	default:
		log.Panicf("Unable to process event")
	}
	return nil
}

// Recv processes incoming requests
func (d *Driver) Recv(req core.Req) *core.Error {
	return nil
}
