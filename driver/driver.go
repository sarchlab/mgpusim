package driver

import (
	"log"

	"gitlab.com/akita/mem/vm"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
)

var HookPosReqStart = &struct{ name string }{"Any"}
var HookPosReqReturn = &struct{ name string }{"Any"}

// Driver is an Akita component that controls the simulated GPUs
type Driver struct {
	*akita.ComponentBase

	engine akita.Engine
	freq   akita.Freq

	gpus        []*gcn3.GPU
	memoryMasks []*MemoryMask
	mmu         vm.MMU
	totalSize   uint64

	usingGPU           int
	currentPID         vm.PID
	PageSizeAsPowerOf2 uint64

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
	req.EndTime = req.Time()
	d.InvokeHook(req, d, HookPosReqReturn, nil)
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

// ChangePID allows the driver to work on another PID in the following API
// calls.
func (d *Driver) ChangePID(pid vm.PID) {
	d.currentPID = pid
}

// NewDriver creates a new driver
func NewDriver(engine akita.Engine, mmu vm.MMU) *Driver {
	driver := new(Driver)
	driver.ComponentBase = akita.NewComponentBase("driver")

	driver.engine = engine
	driver.mmu = mmu
	driver.freq = 1 * akita.GHz
	driver.memoryMasks = make([]*MemoryMask, 0)
	driver.PageSizeAsPowerOf2 = 12

	driver.currentPID = 1

	driver.ToGPUs = akita.NewPort(driver)

	return driver
}
