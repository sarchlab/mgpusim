package driver

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

// HookPosReqStart is a hook position that triggers hook when a request starts
var HookPosReqStart = &struct{ name string }{"Any"}

// HookPosReqReturn is a hook position that triggers hook when a request returns
// to the driver.
var HookPosReqReturn = &struct{ name string }{"Any"}

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.TickingComponent

	gpus        []*gcn3.GPU
	memoryMasks []*MemoryMask
	totalSize   uint64
	usingGPU    int

	CommandQueues []*CommandQueue

	ToGPUs akita.Port
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e akita.Event) error {
	switch e := e.(type) {
	case akita.TickEvent:
		d.handleTickEvent(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (d *Driver) handleTickEvent(evt akita.TickEvent) {
	now := evt.Time()
	d.NeedTick = false

	d.processReturnReq(now)
	d.processNewCommand(now)

	if d.NeedTick {
		d.TickLater(now)
	}
}

func (d *Driver) processReturnReq(now akita.VTimeInSec) {
	req := d.ToGPUs.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *gcn3.MemCopyH2DReq:
		d.processMemCopyH2DReturn(now, req)

	case *gcn3.MemCopyD2HReq:
		d.processMemCopyD2HReturn(now, req)

	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}
}

func (d *Driver) findCommandByReq(req akita.Req) (Command, *CommandQueue) {
	for _, cmdQueue := range d.CommandQueues {
		if len(cmdQueue.Commands) == 0 {
			continue
		}

		if cmdQueue.Commands[0].GetReq() == req {
			return cmdQueue.Commands[0], cmdQueue
		}
	}

	panic("cannot find command")
}

func (d *Driver) processNewCommand(now akita.VTimeInSec) {
	for _, cmdQueue := range d.CommandQueues {
		if len(cmdQueue.Commands) == 0 {
			continue
		}

		if cmdQueue.IsRunning {
			continue
		}

		d.processOneCommand(now, cmdQueue)
	}
}

func (d *Driver) processOneCommand(
	now akita.VTimeInSec,
	cmdQueue *CommandQueue,
) {
	cmd := cmdQueue.Commands[0]

	switch cmd := cmd.(type) {
	case *MemCopyH2DCommand:
		d.processMemCopyH2DCommand(now, cmd, cmdQueue)
	default:
		log.Panicf("cannot process command of type %s", reflect.TypeOf(cmd))
	}
}

func (d *Driver) handleLaunchKernelReq(req *gcn3.LaunchKernelReq) error {
	req.EndTime = req.Time()
	d.InvokeHook(req, d, HookPosReqReturn, nil)
	return nil
}

// RegisterGPU tells the driver about the existance of a GPU
func (d *Driver) RegisterGPU(gpu *gcn3.GPU) {
	d.gpus = append(d.gpus, gpu)

	d.registerStorage(gpu.DRAMStorage, GPUPtr(d.totalSize), gpu.DRAMStorage.Capacity)
	d.totalSize += gpu.DRAMStorage.Capacity
}

// SelectGPU requires the driver to perform the following APIs on a selected
// GPU
func (d *Driver) SelectGPU(gpuID int) {
	if gpuID >= len(d.gpus) {
		log.Panicf("no GPU %d in the system", gpuID)
	}
	d.usingGPU = gpuID
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine) *Driver {
	driver := new(Driver)
	driver.TickingComponent = akita.NewTickingComponent(
		"driver", engine, 1*akita.GHz, driver)

	driver.memoryMasks = make([]*MemoryMask, 0)

	driver.ToGPUs = akita.NewLimitNumReqPort(driver, 1)

	return driver
}
