package driver

import (
	// 	"fmt"

	"fmt"

	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/tracing"
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
	devices     map[string]*gpu.GPUController
	freeDevices []*gpu.GPUController

	// trace kernel
	undispatchedKernels    []*trace.KernelTrace
	unfinishedKernelsCount uint64

	simulationID string
}

func NewDriver(name string, engine sim.Engine, freq sim.Freq) *Driver {
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, engine, freq, d)
	d.toDevices = sim.NewPort(d, 4096, 4096, fmt.Sprintf("%s.ToDevice", name))
	d.AddPort(fmt.Sprintf("%s.ToDevice", name), d.toDevices)

	d.connectionWithDevices = directconnection.MakeBuilder().
		WithEngine(d.Engine).
		WithFreq(1 * sim.GHz).
		Build("DriverToDevice")
	d.connectionWithDevices.PlugIn(d.toDevices)
	d.devices = make(map[string]*gpu.GPUController)

	return d
}

// Run starts the driver and logs simulation start
func (d *Driver) Run() {
	d.logSimulationStart()
}

// Terminate stops the driver and logs simulation termination
func (d *Driver) Terminate() {
	d.logSimulationTerminate()
}

func (d *Driver) logSimulationStart() {
	d.simulationID = xid.New().String()
	tracing.StartTask(
		d.simulationID,
		"",
		d,
		"Simulation", "Simulation",
		nil,
	)
}

func (d *Driver) logSimulationTerminate() {
	tracing.EndTask(d.simulationID, d)
}

func (d *Driver) RegisterGPU(gpu *gpu.GPUController) {
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
	// fmt.Printf("Dispatching kernel %s to device %s at time %.10f\n",
	// 	kernel.ID, d.freeDevices[0].ID, d.Engine.CurrentTime())
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

// func (d *Driver) LogSimulationStart() {
// 	d.simulationID = xid.New().String()
// 	// fmt.Printf("tracing.StartTask: Simulation ID: %s\n", d.simulationID)
// 	tracing.StartTask(d.simulationID, "", d, "Simulation", "Simulation", nil)
// }

// func (d *Driver) LogSimulationTerminate() {
// 	// fmt.Printf("tracing.EndTask: Simulation ID: %s\n", d.simulationID)
// 	tracing.EndTask(d.simulationID, d)
// }

// func (d *Driver) logTaskToGPUInitiate(
// 	cmd Command,
// 	req sim.Msg,
// ) {
// 	tracing.TraceReqInitiate(req, d, cmd.GetID())
// }

// func (d *Driver) logTaskToGPUClear(
// 	req sim.Msg,
// ) {
// 	tracing.TraceReqFinalize(req, d)
// }
