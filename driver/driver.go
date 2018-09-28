package driver

import (
	"fmt"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.ComponentBase

	engine akita.Engine
	freq   akita.Freq

	gpus                     []*gcn3.GPU
	memoryMasks              []*MemoryMask
	totalSize                uint64
	kernelLaunchingStartTime map[string]akita.VTimeInSec
	usingGPU                 int

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
	case *gcn3.LaunchKernelReq:
		return d.handleLaunchKernelReq(e)
	default:
		// Do nothing
	}
	return nil
}

func (d *Driver) handleLaunchKernelReq(req *gcn3.LaunchKernelReq) error {
	startTime := d.kernelLaunchingStartTime[req.ID]
	endTime := req.Time()
	fmt.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
	return nil
}

func (d *Driver) RegisterGPU(gpu *gcn3.GPU) {
	d.gpus = append(d.gpus, gpu)

	d.registerStorage(gpu.DRAMStorage, GPUPtr(d.totalSize), gpu.DRAMStorage.Capacity)
	d.totalSize += gpu.DRAMStorage.Capacity
}

func (d *Driver) SelectGPU(gpuID int) {
	if gpuID >= len(d.gpus) {
		log.Fatalf("no GPU %d in the system", gpuID)
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
	driver.kernelLaunchingStartTime = make(map[string]akita.VTimeInSec)

	driver.ToGPUs = akita.NewPort(driver)

	return driver
}
