package driver

import (
	"log"

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
	*akita.ComponentBase

	engine akita.Engine
	freq   akita.Freq

	gpus        []*gcn3.GPU
	memoryMasks []*MemoryMask
	totalSize   uint64
	usingGPU    int

	CommandQueues []*CommandQueue

	ToGPUs akita.Port
}

// NotifyPortFree of the Driver component does nothing.
func (d *Driver) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	// Do nothing
}

// NotifyRecv of the Driver component converts requests as event and schedules
// them.
func (d *Driver) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, d.engine, d.freq)
}

// Handle process event that is scheduled on the driver
func (d *Driver) Handle(e akita.Event) error {
	switch e := e.(type) {
	case *gcn3.LaunchKernelReq:
		return d.handleLaunchKernelReq(e)
	default:
		// Do nothing
	}
	return nil
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
	driver.ComponentBase = akita.NewComponentBase("driver")

	driver.engine = engine
	driver.freq = 1 * akita.GHz
	driver.memoryMasks = make([]*MemoryMask, 0)

	driver.ToGPUs = akita.NewLimitNumReqPort(driver, 1)

	return driver
}
