package driver

import (
	"fmt"

	"github.com/sarchlab/accelsimtracing/gpu"
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/accelsimtracing/nvidia"
	"github.com/sarchlab/akita/v3/sim"
)

type Driver struct {
	*sim.TickingComponent

	toDevices             sim.Port
	connectionWithDevices sim.Connection

	// gpu
	devices     map[string]*gpu.GPU
	freeDevices []*gpu.GPU

	// trace kernel
	undispatchedKernels    []*nvidia.Kernel
	unfinishedKernelsCount int64
}

func NewDriver(name string, engine sim.Engine, freq sim.Freq) *Driver {
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, engine, freq, d)
	d.toDevices = sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	d.AddPort("ToDevice", d.toDevices)

	d.connectionWithDevices = sim.NewDirectConnection("DriverToDevice", d.Engine, 1*sim.GHz)
	d.connectionWithDevices.PlugIn(d.toDevices, 4)

	return d
}

func (d *Driver) RegisterGPU(gpu *gpu.GPU) {
	gpu.SetDriverRemotePort(d.toDevices)
	remote := gpu.GetPortByName("ToDriver")
	d.connectionWithDevices.PlugIn(remote, 4)

	d.devices[gpu.ID] = gpu
	d.freeDevices = append(d.freeDevices, gpu)
}

func (d *Driver) RunKernel(kernel *nvidia.Kernel) {
	d.undispatchedKernels = append(d.undispatchedKernels, kernel)
	d.unfinishedKernelsCount++
}

func (d *Driver) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = d.dispatchKernelsToDevices(now) || madeProgress
	madeProgress = d.processDevicesInput(now) || madeProgress

	// fmt.Println("Driver tick, madeProgress:", madeProgress)

	return madeProgress
}

func (d *Driver) processDevicesInput(now sim.VTimeInSec) bool {
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
	if msg.KernelFinished {
		d.freeDevices = append(d.freeDevices, d.devices[msg.DeviceID])
		d.unfinishedKernelsCount--
		if d.unfinishedKernelsCount == 0 {
			fmt.Println("All kernels finished, time is:", now)
		}
	}

	d.toDevices.Retrieve(now)
}

func (d *Driver) dispatchKernelsToDevices(now sim.VTimeInSec) bool {
	if len(d.undispatchedKernels) == 0 || len(d.freeDevices) == 0 {
		return false
	}

	kernel := d.undispatchedKernels[0]
	device := d.freeDevices[0]

	msg := &message.DriverToDeviceMsg{
		Kernel: *kernel,
	}
	msg.Src = d.toDevices
	msg.Dst = device.GetPortByName("ToDriver")
	msg.SendTime = now

	err := d.toDevices.Send(msg)
	if err != nil {
		return false
	}

	d.undispatchedKernels = d.undispatchedKernels[1:]
	d.freeDevices = d.freeDevices[1:]

	return true
}
