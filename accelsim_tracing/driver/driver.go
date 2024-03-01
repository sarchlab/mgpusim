package driver

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/gpu"
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/akita/v3/sim"
)

type Driver struct {
	*sim.TickingComponent
	ID int64

	toDevices             sim.Port
	connectionWithDevices sim.Connection

	// gpu
	deviceCount             int64
	devices                 []DeviceInfo
	freeDeviceIndex         []int64
	requestMoreThreadblocks []int64

	// trace kernel
	kernelCount   int64
	kernels       []KernelInfo
	waitingKernel []int64
}

type DeviceInfo struct {
	device *gpu.GPU

	toDeviceRemote sim.Port

	kernelID int64
}

type KernelInfo struct {
	kernel   benchmark.Kernel
	finished bool

	nextThreadblockToRun int64
	finishedTBCount      int64
}

func (d *Driver) RegisterGPU(gpu *gpu.GPU) {
	newDevice := &DeviceInfo{
		device: gpu,
	}

	toDeviceRemote := gpu.GetPortByName("ToDriver")
	d.connectionWithDevices.PlugIn(toDeviceRemote, 1)

	newDevice.toDeviceRemote = toDeviceRemote

	d.devices = append(d.devices, *newDevice)
	d.freeDeviceIndex = append(d.freeDeviceIndex, d.deviceCount)
	d.deviceCount++
}

func (d *Driver) RunKernel(kernel benchmark.Kernel) {
	newKernel := &KernelInfo{
		kernel:               kernel,
		finished:             false,
		nextThreadblockToRun: 0,
		finishedTBCount:      0,
	}

	d.waitingKernel = append(d.waitingKernel, d.kernelCount)
	d.kernels = append(d.kernels, *newKernel)
	d.kernelCount++
}

func (d *Driver) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = d.applyKernelToDevices(now) || madeProgress
	madeProgress = d.addThreadBlocksToDevices(now) || madeProgress
	madeProgress = d.processInput(now) || madeProgress
	madeProgress = d.divicesUnfinished(now) || madeProgress

	return madeProgress
}

func (d *Driver) processInput(now sim.VTimeInSec) bool {
	msg := d.toDevices.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToDriverMsg:
		d.processDeviceMsg(msg, now)
		return true
	default:
		panic("Unknown message type")
	}
}

func (d *Driver) processDeviceMsg(msg *message.DeviceToDriverMsg, now sim.VTimeInSec) {
	if msg.RequestMore {
		d.requestMoreThreadblocks = append(d.requestMoreThreadblocks, msg.DeviceID)
		d.devices[msg.DeviceID].toDeviceRemote.Retrieve(now)

		return
	}

	if msg.ThreadblockFinished {
		kernel := &d.kernels[d.devices[msg.DeviceID].kernelID]
		kernel.finishedTBCount++
		if kernel.finishedTBCount == kernel.kernel.ThreadblocksCount {
			kernel.finished = true
			d.freeDeviceIndex = append(d.freeDeviceIndex, msg.DeviceID)
		}
	}

	d.devices[msg.DeviceID].toDeviceRemote.Retrieve(now)
}

func (d *Driver) applyKernelToDevices(now sim.VTimeInSec) bool {
	if len(d.waitingKernel) == 0 || len(d.freeDeviceIndex) == 0 {
		return false
	}

	kernelID := d.waitingKernel[0]
	deviceID := d.freeDeviceIndex[0]
	kernel := &d.kernels[kernelID]
	device := &d.devices[deviceID]

	msg := &message.DriverToDeviceMsg{
		NewKernel: true,
	}
	msg.Src = d.toDevices
	msg.Dst = device.toDeviceRemote
	msg.SendTime = now

	err := device.toDeviceRemote.Send(msg)
	if err != nil {
		return false
	}

	// d.Engine.Pause()
	// device.device.TickLater(d.Engine.CurrentTime())
	// d.Engine.Continue()

	kernel.nextThreadblockToRun = 0
	kernel.finishedTBCount = 0
	device.kernelID = kernelID

	d.waitingKernel = d.waitingKernel[1:]
	d.freeDeviceIndex = d.freeDeviceIndex[1:]

	return true
}

func (d *Driver) addThreadBlocksToDevices(now sim.VTimeInSec) bool {
	if d.requestMoreThreadblocks == nil {
		return false
	}

	deviceID := d.requestMoreThreadblocks[0]
	kernelID := d.devices[deviceID].kernelID
	device := &d.devices[deviceID]
	kernel := &d.kernels[kernelID]

	if kernel.nextThreadblockToRun == kernel.kernel.ThreadblocksCount {
		return false
	}

	msg := &message.DriverToDeviceMsg{
		NewKernel:   false,
		Threadblock: kernel.kernel.Threadblocks[kernel.nextThreadblockToRun],
	}
	msg.Src = d.toDevices
	msg.Dst = device.toDeviceRemote
	msg.SendTime = now

	err := device.toDeviceRemote.Send(msg)
	if err != nil {
		return false
	}

	kernel.nextThreadblockToRun++

	d.requestMoreThreadblocks = d.requestMoreThreadblocks[1:]

	return true
}

func (d *Driver) divicesUnfinished(now sim.VTimeInSec) bool {
	return len(d.freeDeviceIndex) != int(d.deviceCount)
}
