package driver

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/gpu"
)

type DriverBuilder struct {
	engine sim.Engine
	freq   sim.Freq
}

func (b *DriverBuilder) WithEngine(engine sim.Engine) *DriverBuilder {
	b.engine = engine
	return b
}

func (b *DriverBuilder) WithFreq(freq sim.Freq) *DriverBuilder {
	b.freq = freq
	return b
}

func (b *DriverBuilder) Build(name string) *Driver {
	d := &Driver{
		devices: make(map[string]*gpu.GPU),
	}

	d.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, d)
	b.buildPortsForDriver(d)

    // v3
    // 	d.connectionWithDevices = sim.NewDirectConnection("ConnWithDevices", b.engine, b.freq)
	d.connectionWithDevices = directconnection.MakeBuilder().
       WithEngine(b.engine).
       WithFreq(b.freq).
       Build("ConnWithDevices")
	// v3
	// d.connectionWithDevices.PlugIn(d.toDevices, 1)
	d.connectionWithDevices.PlugIn(d.toDevices)

	return d
}

func (b *DriverBuilder) buildPortsForDriver(d *Driver) {
    // v3
    // d.toDevices = sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	d.toDevices = sim.NewPort(d, 4, 4, "ToDevice")
	d.AddPort("ToDevice", d.toDevices)
}
