package component

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
)

type Driver struct {
	*sim.TickingComponent

	status []*StatusRecord

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

type StatusRecord struct {
	component sim.Component
	property  ReportProperties
	value     string
}

type DeviceInfo struct {
	device *GPU

	toDeviceSrc sim.Port
	toDeviceDst sim.Port

	kernelID int64
}

type KernelInfo struct {
	kernel   Kernel
	finished bool

	nextThreadblockToRun int64
	finishedTBCount      int64

	startTime sim.VTimeInSec
	endTime   sim.VTimeInSec
}

func NewDriver(name string, engine sim.Engine, freq sim.Freq) *Driver {
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, engine, freq, d)
	return d
}

func (d *Driver) RegisterGPU(gpu *GPU) {
	newDevice := &DeviceInfo{
		device: gpu,
	}

	toGPUSrc := sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	toGPUDst := gpu.toDriverSrc

	conn := sim.NewDirectConnection("Conn", d.Engine, d.Freq)
	conn.PlugIn(toGPUSrc, 1)
	conn.PlugIn(toGPUDst, 1)

	newDevice.toDeviceDst = toGPUDst
	newDevice.toDeviceSrc = toGPUSrc
	gpu.toDriverDst = toGPUSrc

	d.devices = append(d.devices, *newDevice)
	d.freeDeviceIndex = append(d.freeDeviceIndex, d.deviceCount)
	d.deviceCount++
}

func (d *Driver) RunKernel(kernel Kernel) {
	newKernel := &KernelInfo{
		kernel:               kernel,
		finished:             false,
		nextThreadblockToRun: 0,
		finishedTBCount:      0,
	}

	d.waitingKernel = append(d.waitingKernel, d.kernelCount)
	d.kernels = append(d.kernels, *newKernel)
	d.kernelCount++

	println("[Driver: RunKernel]")
}

func (d *Driver) Tick(now sim.VTimeInSec) bool {
	println("[Driver: Tick]")

	madeProgress := false

	madeProgress = d.applyKernelToDevices(now) || madeProgress
	madeProgress = d.addThreadBlocksToDevices(now) || madeProgress
	madeProgress = d.processInput(now) || madeProgress

	return madeProgress
}

// DriverToDeviceMsg: apply a new kernel to a device or answer a device request for more threadblocks
type DriverToDeviceMsg struct {
	sim.MsgMeta

	newKernel   bool
	threadblock Threadblock
}

func (m *DriverToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// DeviceToDriverMsg: report a finished threadblock or request more threadblocks
type DeviceToDriverMsg struct {
	sim.MsgMeta

	requestMore         bool
	threadBlockFinished bool
}

func (m *DeviceToDriverMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (d *Driver) processInput(now sim.VTimeInSec) bool {
	for i := int64(0); i < d.deviceCount; i++ {
		device := &d.devices[i]
		msg := device.toDeviceSrc.Peek()
		if msg == nil {
			continue
		}

		switch msg := msg.(type) {
		case *DeviceToDriverMsg:
			d.processDeviceMsg(msg, now, i)
			return true
		default:
			panic("Unknown message type")
		}
	}

	return false

}

func (d *Driver) processDeviceMsg(msg *DeviceToDriverMsg, now sim.VTimeInSec, deviceID int64) {
	if msg.requestMore {
		d.requestMoreThreadblocks = append(d.requestMoreThreadblocks, deviceID)
		d.devices[deviceID].toDeviceSrc.Retrieve(now)

		return
	}

	if msg.threadBlockFinished {
		kernel := &d.kernels[d.devices[deviceID].kernelID]
		kernel.finishedTBCount++
		if kernel.finishedTBCount == kernel.kernel.ThreadblocksCount() {
			kernel.finished = true
			kernel.endTime = now
			d.freeDeviceIndex = append(d.freeDeviceIndex, deviceID)
		}
	}

	d.devices[deviceID].toDeviceSrc.Retrieve(now)
}

func (d *Driver) applyKernelToDevices(now sim.VTimeInSec) bool {
	if len(d.waitingKernel) == 0 || len(d.freeDeviceIndex) == 0 {
		return false
	}

	kernelID := d.waitingKernel[0]
	deviceID := d.freeDeviceIndex[0]
	kernel := &d.kernels[kernelID]
	device := &d.devices[deviceID]

	msg := &DriverToDeviceMsg{
		newKernel: true,
	}
	msg.Src = device.toDeviceSrc
	msg.Dst = device.toDeviceDst
	msg.SendTime = now

	err := device.toDeviceSrc.Send(msg)
	if err != nil {
		return false
	}

	kernel.nextThreadblockToRun = 0
	kernel.finishedTBCount = 0
	kernel.startTime = now
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

	if kernel.nextThreadblockToRun == kernel.kernel.ThreadblocksCount() {
		return false
	}

	msg := &DriverToDeviceMsg{
		newKernel:   false,
		threadblock: *kernel.kernel.Threadblock(kernel.nextThreadblockToRun),
	}
	msg.Src = device.toDeviceSrc
	msg.Dst = device.toDeviceDst
	msg.SendTime = now

	err := device.toDeviceSrc.Send(msg)
	if err != nil {
		return false
	}

	kernel.nextThreadblockToRun++

	d.requestMoreThreadblocks = d.requestMoreThreadblocks[1:]

	return true
}

func (d *Driver) ReportStatus(property ReportProperties) {
	for _, status := range d.status {
		if status.property == property {
			fmt.Printf("[%s # %s] : %s\n",
				status.component.Name(), string(property), status.value)
		}
	}
}
