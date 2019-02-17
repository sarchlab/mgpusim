package driver

import (
	"log"
	"sync/atomic"

	"gitlab.com/akita/mem/vm"
)

var nextPID uint64

func (d *Driver) Init() *Context {
	atomic.AddUint64(&nextPID, 1)

	return &Context{
		PID:          vm.PID(nextPID),
		CurrentGPUID: 0,
	}
}

// GetNumGPUs return the number of GPUs in the platform
func (d *Driver) GetNumGPUs(c *Context) int {
	return len(d.gpus)
}

// SelectGPU requires the driver to perform the following APIs on a selected
// GPU
func (d *Driver) SelectGPU(c *Context, gpuID int) {
	if gpuID >= len(d.gpus) {
		log.Panicf("GPU %d is not available", gpuID)
	}
	c.CurrentGPUID = gpuID
}
