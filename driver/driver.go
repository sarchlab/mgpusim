package driver

import (
	"fmt"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"
)

// Driver is an Yaotsu component that controls the simulated GPUs
type Driver struct {
	*core.ComponentBase

	engine core.Engine
	freq   core.Freq

	memoryMasks              map[*mem.Storage]*MemoryMask
	kernelLaunchingStartTime map[string]core.VTimeInSec

	ToGPUs *core.Port
}

func (d *Driver) NotifyPortFree(now core.VTimeInSec, port *core.Port) {
	// Do nothing
}

func (d *Driver) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	req := port.Retrieve(now)
	core.ProcessReqAsEvent(req, d.engine, d.freq)
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e core.Event) error {
	switch e := e.(type) {
	case *kernels.LaunchKernelReq:
		return d.handleLaunchKernelReq(e)
	default:
		// Do nothing
	}
	return nil
}

func (d *Driver) handleLaunchKernelReq(req *kernels.LaunchKernelReq) error {
	startTime := d.kernelLaunchingStartTime[req.ID]
	endTime := req.Time()
	fmt.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
	return nil
}

// NewDriver creates a new driver
func NewDriver(engine core.Engine) *Driver {
	driver := new(Driver)
	driver.ComponentBase = core.NewComponentBase("driver")

	driver.engine = engine
	driver.freq = 1 * core.GHz
	driver.memoryMasks = make(map[*mem.Storage]*MemoryMask)
	driver.kernelLaunchingStartTime = make(map[string]core.VTimeInSec)

	driver.ToGPUs = core.NewPort(driver)

	return driver
}
