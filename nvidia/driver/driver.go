package driver

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/gpu"
	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const portBufSize = 4096

// Driver models the CUDA driver. It launches the traced kernels, in order,
// on the free GPUs, and records when the last kernel finishes.
type Driver struct {
	*modeling.TickingComponent

	toDevices             messaging.Port
	connectionWithDevices *directconnection.Comp

	devices     map[string]*gpu.GPUController
	freeDevices []*gpu.GPUController

	undispatchedKernels    []*trace.KernelTrace
	unfinishedKernelsCount uint64

	// Cycles the driver waits between two kernel launches.
	launchOverheadLatency          uint64
	launchOverheadLatencyRemaining uint64

	finishTime timing.VTimeInPicoSec
}

// Builder builds a Driver.
type Builder struct {
	registrar             modeling.Registrar
	freq                  timing.Freq
	launchOverheadLatency uint64
}

// MakeBuilder creates a driver builder.
func MakeBuilder() Builder {
	return Builder{freq: 1 * timing.GHz}
}

// WithRegistrar sets the simulation that the driver registers to.
func (b Builder) WithRegistrar(r modeling.Registrar) Builder {
	b.registrar = r
	return b
}

// WithFreq sets the frequency of the driver.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithLaunchOverheadLatency sets the cycles the driver waits between two
// kernel launches.
func (b Builder) WithLaunchOverheadLatency(latency uint64) Builder {
	b.launchOverheadLatency = latency
	return b
}

// Build creates a driver with the given name.
func (b Builder) Build(name string) *Driver {
	d := &Driver{
		devices:                        make(map[string]*gpu.GPUController),
		launchOverheadLatency:          b.launchOverheadLatency,
		launchOverheadLatencyRemaining: b.launchOverheadLatency,
	}
	d.TickingComponent = modeling.NewTickingComponent(
		name, b.registrar.GetEngine(), b.freq, d)
	b.registrar.RegisterComponent(d)

	d.DeclarePort("ToDevices")
	d.toDevices = modeling.MakePortBuilder().
		WithRegistrar(b.registrar).
		WithComponent(d).
		WithSpec(modeling.PortSpec{BufSize: portBufSize}).
		Build("ToDevices")
	d.AssignPort("ToDevices", d.toDevices)

	d.connectionWithDevices = directconnection.MakeBuilder().
		WithRegistrar(b.registrar).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(name + ".ConnWithDevices")
	d.connectionWithDevices.PlugIn(d.toDevices)

	return d
}

// RegisterGPU connects a GPU to the driver.
func (d *Driver) RegisterGPU(g *gpu.GPUController) {
	g.SetDriverRemotePort(d.toDevices.AsRemote())
	d.connectionWithDevices.PlugIn(g.GetPortByName("ToDriver"))

	d.devices[g.ID] = g
	d.freeDevices = append(d.freeDevices, g)
}

// RunKernel queues a kernel for execution.
func (d *Driver) RunKernel(kernel *trace.KernelTrace) {
	d.undispatchedKernels = append(d.undispatchedKernels, kernel)
	d.unfinishedKernelsCount++
}

// FinishTime returns the time when the last kernel finished.
func (d *Driver) FinishTime() timing.VTimeInPicoSec {
	return d.finishTime
}

// Tick advances the driver by one cycle.
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
	case message.DeviceToDriverMsg:
		d.processDeviceMsg(msg)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from device",
			d.Name(), msg))
	}

	d.toDevices.RetrieveIncoming()

	return true
}

func (d *Driver) processDeviceMsg(msg message.DeviceToDriverMsg) {
	if !msg.KernelFinished {
		return
	}

	d.freeDevices = append(d.freeDevices, d.devices[msg.DeviceID])

	d.unfinishedKernelsCount--
	if d.unfinishedKernelsCount == 0 {
		d.finishTime = d.CurrentTime()
	}
}

func (d *Driver) dispatchKernelsToDevices() bool {
	if len(d.undispatchedKernels) == 0 || len(d.freeDevices) == 0 {
		return false
	}

	if d.launchOverheadLatencyRemaining > 0 {
		d.launchOverheadLatencyRemaining--
		return true
	}

	if !d.toDevices.CanSend() {
		return false
	}

	device := d.freeDevices[0]
	dst := device.GetPortByName("ToDriver").AsRemote()
	d.toDevices.Send(message.DriverToDeviceMsg{
		MsgMeta: message.NewMeta(d.toDevices.AsRemote(), dst),
		Kernel:  d.undispatchedKernels[0],
	})

	d.undispatchedKernels = d.undispatchedKernels[1:]
	d.freeDevices = d.freeDevices[1:]
	d.launchOverheadLatencyRemaining = d.launchOverheadLatency

	return true
}
