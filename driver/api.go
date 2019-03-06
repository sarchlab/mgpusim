package driver

import (
	"fmt"
	"log"
	"sync/atomic"

	"gitlab.com/akita/mem/vm"
)

var nextPID uint64

func (d *Driver) Init() *Context {
	atomic.AddUint64(&nextPID, 1)

	c := &Context{
		PID:          vm.PID(nextPID),
		CurrentGPUID: 0,
	}
	d.Contexts = append(d.Contexts, c)

	return c
}

// GetNumGPUs return the number of GPUs in the platform
func (d *Driver) GetNumGPUs() int {
	return len(d.GPUs)
}

// SelectGPU requires the driver to perform the following APIs on a selected
// GPU
func (d *Driver) SelectGPU(c *Context, gpuID int) {
	if gpuID >= len(d.GPUs) {
		log.Panicf("GPU %d is not available", gpuID)
	}
	c.CurrentGPUID = gpuID
}

// CreateCommandQueue creates a command queue in the driver
func (d *Driver) CreateCommandQueue(c *Context) *CommandQueue {
	q := new(CommandQueue)
	q.GPUID = c.CurrentGPUID
	q.Context = c

	c.Queues = append(c.Queues, q)

	return q
}

// DrainCommandQueue will return when there is no command to execute
func (d *Driver) DrainCommandQueue(q *CommandQueue) {
	listener := q.Subscribe()
	defer q.Unsubscribe(listener)
	for {
		if len(q.Commands) == 0 {
			return
		}
		fmt.Printf("wait for drain signal, commands left %d\n", len(q.Commands))
		listener.Wait()
	}
}
