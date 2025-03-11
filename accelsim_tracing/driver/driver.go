package driver

import (
    // 	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/gpu"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/message"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"

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
	undispatchedKernels    []*nvidia.Kernel
	unfinishedKernelsCount int64
}

func NewDriver(name string, engine sim.Engine, freq sim.Freq) *Driver {
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, engine, freq, d)
	// v3
	// d.toDevices = sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	d.toDevices = sim.NewPort(d, 4, 4, "ToDevice")
	d.AddPort("ToDevice", d.toDevices)

    // v3
    // 	d.connectionWithDevices = sim.NewDirectConnection("DriverToDevice", d.Engine, 1*sim.GHz)
    d.connectionWithDevices = directconnection.MakeBuilder().
       WithEngine(d.Engine).
       WithFreq(1*sim.GHz).
       Build("DriverToDevice")
	// v3
	// d.connectionWithDevices.PlugIn(d.toDevices, 4)
	d.connectionWithDevices.PlugIn(d.toDevices)

	return d
}

func (d *Driver) RegisterGPU(gpu *gpu.GPU) {
	gpu.SetDriverRemotePort(d.toDevices)
	remote := gpu.GetPortByName("ToDriver")
	// v3
	// d.connectionWithDevices.PlugIn(remote, 4)
	d.connectionWithDevices.PlugIn(remote)

	d.devices[gpu.ID] = gpu
	d.freeDevices = append(d.freeDevices, gpu)
}

func (d *Driver) RunKernel(kernel *nvidia.Kernel) {
	d.undispatchedKernels = append(d.undispatchedKernels, kernel)
	d.unfinishedKernelsCount++
}

// v3
// func (d *Driver) Tick(now sim.VTimeInSec) bool {
func (d *Driver) Tick() bool {
	madeProgress := false
    // v3
    // madeProgress = d.dispatchKernelsToDevices(now) || madeProgress
    // madeProgress = d.processDevicesInput(now) || madeProgress
	madeProgress = d.dispatchKernelsToDevices() || madeProgress
	madeProgress = d.processDevicesInput() || madeProgress

	return madeProgress
}

// v3
// func (d *Driver) processDevicesInput(now sim.VTimeInSec) bool {
func (d *Driver) processDevicesInput() bool {
    // v3
    // msg := d.toDevices.Peek()
	msg := d.toDevices.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToDriverMsg:
		d.processDeviceMsg(msg)
		// v3
		// d.processDeviceMsg(msg, now)
	default:
		log.WithField("function", "processDevicesInput").Panic("Unknown message type")
	}

	return true
}

// v3
// func (d *Driver) processDeviceMsg(msg *message.DeviceToDriverMsg, now sim.VTimeInSec) {
func (d *Driver) processDeviceMsg(msg *message.DeviceToDriverMsg) {
	if msg.KernelFinished {
		d.freeDevices = append(d.freeDevices, d.devices[msg.DeviceID])
		d.unfinishedKernelsCount--
		if d.unfinishedKernelsCount == 0 {
		    // v3
		    // log.WithField("time", now).Info("All kernels finished")
			log.WithField("time", "1234").Info("All kernels finished")
			// v3
            // fmt.Println(now)
		}
	}
    // v3
    // d.toDevices.Retrieve(now)
	d.toDevices.RetrieveIncoming()
}

// v3
// func (d *Driver) dispatchKernelsToDevices(now sim.VTimeInSec) bool {
func (d *Driver) dispatchKernelsToDevices() bool {
	if len(d.undispatchedKernels) == 0 || len(d.freeDevices) == 0 {
		return false
	}

	kernel := d.undispatchedKernels[0]
	device := d.freeDevices[0]

	msg := &message.DriverToDeviceMsg{
		Kernel: *kernel,
	}
	// v3
	// msg.Src = d.toDevices
	// msg.Dst = device.GetPortByName("ToDriver")
	msg.Src = d.toDevices.AsRemote()
	msg.Dst = device.GetPortByName("ToDriver").AsRemote()
	// v3
    // 	msg.SendTime = now

	err := d.toDevices.Send(msg)
	if err != nil {
		return false
	}

	d.undispatchedKernels = d.undispatchedKernels[1:]
	d.freeDevices = d.freeDevices[1:]

	return true
}
