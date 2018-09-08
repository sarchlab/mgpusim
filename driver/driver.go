package driver

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/mem"
)

// Driver is an Yaotsu component that controls the simulated GPUs
type Driver struct {
	*akita.ComponentBase

	engine akita.Engine
	freq   akita.Freq

	memoryMasks              map[*mem.Storage]*MemoryMask
	kernelLaunchingStartTime map[string]akita.VTimeInSec

	ToGPUs *akita.Port
}

func (d *Driver) NotifyPortFree(now akita.VTimeInSec, port *akita.Port) {
	// Do nothing
}

func (d *Driver) NotifyRecv(now akita.VTimeInSec, port *akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, d.engine, d.freq)
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e akita.Event) error {
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
func NewDriver(engine akita.Engine) *Driver {
	driver := new(Driver)
	driver.ComponentBase = akita.NewComponentBase("driver")

	driver.engine = engine
	driver.freq = 1 * akita.GHz
	driver.memoryMasks = make(map[*mem.Storage]*MemoryMask)
	driver.kernelLaunchingStartTime = make(map[string]akita.VTimeInSec)

	driver.ToGPUs = akita.NewPort(driver)

	return driver
}
