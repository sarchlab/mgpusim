package driver

import (
	// 	"fmt"

	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"

	log "github.com/sirupsen/logrus"
)

type Driver struct {
	*sim.TickingComponent

	toDevices             sim.Port
	connectionWithDevices sim.Connection

	// gpu
	devices     map[string]*gpu.GPU
	freeDevices []*gpu.GPU

	// trace kernel
	undispatchedKernels    []*trace.KernelTrace
	unfinishedKernelsCount uint64
}

func NewDriver(name string, engine sim.Engine, freq sim.Freq) *Driver {
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, engine, freq, d)
	d.toDevices = sim.NewPort(d, 4, 4, fmt.Sprintf("%s.ToDevice", name))
	d.AddPort(fmt.Sprintf("%s.ToDevice", name), d.toDevices)

	d.connectionWithDevices = directconnection.MakeBuilder().
		WithEngine(d.Engine).
		WithFreq(1 * sim.GHz).
		Build("DriverToDevice")
	d.connectionWithDevices.PlugIn(d.toDevices)

	return d
}

func (d *Driver) RegisterGPU(gpu *gpu.GPU) {
	gpu.SetDriverRemotePort(d.toDevices)
	remote := gpu.GetPortByName(fmt.Sprintf("%s.ToDriver", gpu.Name()))
	d.connectionWithDevices.PlugIn(remote)

	d.devices[gpu.ID] = gpu
	d.freeDevices = append(d.freeDevices, gpu)
}

func (d *Driver) RunKernel(kernel *trace.KernelTrace) {
	d.undispatchedKernels = append(d.undispatchedKernels, kernel)
	d.unfinishedKernelsCount++
}

func (d *Driver) Tick() bool {
	madeProgress := false
	madeProgress = d.dispatchKernelsToDevices() || madeProgress
	madeProgress = d.processDevicesInput() || madeProgress

	return madeProgress
}

func (d *Driver) processDevicesInput() bool {
	msg := d.toDevices.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToDriverMsg:
		d.processDeviceMsg(msg)
	default:
		log.WithField("function", "processDevicesInput").Panic("Unknown message type")
	}

	return true
}

func (d *Driver) processDeviceMsg(msg *message.DeviceToDriverMsg) {
	now := d.Engine.CurrentTime()
	if msg.KernelFinished {
		d.freeDevices = append(d.freeDevices, d.devices[msg.DeviceID])
		d.unfinishedKernelsCount--
		if d.unfinishedKernelsCount == 0 {
			log.WithField("time", now).Info("All kernels finished")
			fmt.Println(now)
		}
	}
	d.toDevices.RetrieveIncoming()
}

func (d *Driver) dispatchKernelsToDevices() bool {
	if len(d.undispatchedKernels) == 0 || len(d.freeDevices) == 0 {
		return false
	}

	kernel := d.undispatchedKernels[0]
	device := d.freeDevices[0]

	msg := &message.DriverToDeviceMsg{
		Kernel: *kernel,
	}
	msg.Src = d.toDevices.AsRemote()
	msg.Dst = device.GetPortByName(fmt.Sprintf("%s.ToDriver", device.Name())).AsRemote()

	err := d.toDevices.Send(msg)
	if err != nil {
		return false
	}

	d.undispatchedKernels = d.undispatchedKernels[1:]
	d.freeDevices = d.freeDevices[1:]

	return true
}
