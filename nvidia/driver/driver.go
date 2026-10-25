package driver

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/gpu"
	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const portBufSize = 4096

// DefaultSpec returns the default component configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

// Driver models the CUDA driver. It launches the traced kernels, in order,
// on the free GPUs, and records when the last kernel finishes.
type Driver struct {
	*Comp
	*topology
}

type Spec struct {
	Freq                  timing.Freq
	LaunchOverheadLatency uint64
}

type State struct {
	FreeDevices                    []string
	UndispatchedKernels            []trace.KernelTrace
	UnfinishedKernelsCount         uint64
	LaunchOverheadLatencyRemaining uint64
	FinishTime                     timing.VTimeInPicoSec
}

type topology struct {
	connectionWithDevices *directconnection.Connection
	devices               map[string]*gpu.GPUController
	deviceOrder           []string
}

type Resources struct{ Topology *topology }
type Ports struct {
	ToDevices messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.driver"`
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Middlewares struct{ Controller *Driver }

func (m *Middlewares) Initialize(c *Comp) {
	m.Controller = &Driver{Comp: c, topology: c.Resources.Topology}
}
func (c *Driver) Handle(timing.Event) bool { return c.Tick() }

// Builder builds a Driver.
type Builder struct {
	registrar             modeling.Simulation
	freq                  timing.Freq
	launchOverheadLatency uint64
}

// MakeBuilder creates a driver builder.
func MakeBuilder() Builder {
	return Builder{freq: DefaultSpec().Freq}
}

// WithSimulation sets the simulation that the driver registers to.
func (b Builder) WithSimulation(r modeling.Simulation) Builder {
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
	t := &topology{devices: make(map[string]*gpu.GPUController)}
	c := modeling.MakeBuilder[*Comp](Spec{Freq: b.freq, LaunchOverheadLatency: b.launchOverheadLatency}).
		WithSimulation(b.registrar).WithResources(Resources{Topology: t}).Build(name)
	d := &Driver{Comp: c, topology: t}
	port := twowaybuffered.MakeBuilder().WithSimulation(b.registrar).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: portBufSize, OutgoingCapacity: portBufSize}).Build()
	d.BindPort("ToDevices", port)
	d.connectionWithDevices = directconnection.NewConnection(name+".ConnWithDevices", b.registrar, b.freq)
	d.connectionWithDevices.BindPort(port)

	return d
}

// RegisterGPU connects a GPU to the driver.
func (d *Driver) RegisterGPU(g *gpu.GPUController) {
	g.SetDriverRemotePort(d.Ports.ToDevices.AsRemote())
	d.connectionWithDevices.BindPort(g.Ports.ToDriver)

	d.devices[g.ID] = g
	d.deviceOrder = append(d.deviceOrder, g.ID)
}

// RunKernel queues a kernel for execution.
func (d *Driver) RunKernel(kernel *trace.KernelTrace) {
	d.State.UndispatchedKernels = append(d.State.UndispatchedKernels, *kernel)
	d.State.UnfinishedKernelsCount++
}

// FinishTime returns the time when the last kernel finished.
func (d *Driver) FinishTime() timing.VTimeInPicoSec {
	return d.State.FinishTime
}

// Tick advances the driver by one cycle.
func (d *Driver) Tick() bool {
	madeProgress := false
	madeProgress = d.dispatchKernelsToDevices() || madeProgress
	madeProgress = d.processDevicesInput() || madeProgress

	return madeProgress
}

func (d *Driver) processDevicesInput() bool {
	msg, ok := d.Ports.ToDevices.PeekIncoming()
	if !ok {
		return false
	}

	switch msg := msg.Payload.(type) {
	case message.DeviceToDriverMsg:
		d.processDeviceMsg(msg)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from device",
			d.Name(), msg))
	}

	d.Ports.ToDevices.RetrieveIncoming()

	return true
}

func (d *Driver) processDeviceMsg(msg message.DeviceToDriverMsg) {
	if !msg.KernelFinished {
		return
	}

	d.State.FreeDevices = append(d.State.FreeDevices, msg.DeviceID)

	d.State.UnfinishedKernelsCount--
	if d.State.UnfinishedKernelsCount == 0 {
		d.State.FinishTime = d.CurrentTime()
	}
}

func (d *Driver) dispatchKernelsToDevices() bool {
	if len(d.State.UndispatchedKernels) == 0 || len(d.State.FreeDevices) == 0 {
		return false
	}

	if d.State.LaunchOverheadLatencyRemaining > 0 {
		d.State.LaunchOverheadLatencyRemaining--
		return true
	}

	if !d.Ports.ToDevices.CanSend() {
		return false
	}

	device := d.devices[d.State.FreeDevices[0]]
	dst := device.Ports.ToDriver.AsRemote()
	d.Ports.ToDevices.Send(
		messaging.Msg{ID: d.NewID(), Src: d.Ports.ToDevices.AsRemote(), Dst: dst, Payload: message.DriverToDeviceMsg{
			Kernel: d.State.UndispatchedKernels[0],
		}},
	)

	d.State.UndispatchedKernels = d.State.UndispatchedKernels[1:]
	d.State.FreeDevices = d.State.FreeDevices[1:]
	d.State.LaunchOverheadLatencyRemaining = d.Spec.LaunchOverheadLatency

	return true
}

// UnfinishedKernels returns the number of kernels that have not finished.
func (d *Driver) UnfinishedKernels() uint64 {
	return d.State.UnfinishedKernelsCount
}

func (s *State) Initialize(c *Comp) {
	s.LaunchOverheadLatencyRemaining = c.Spec.LaunchOverheadLatency
	for _, id := range c.Resources.Topology.deviceOrder {
		s.FreeDevices = append(s.FreeDevices, id)
	}
}
