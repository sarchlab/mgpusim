package driver

import (
	"github.com/sarchlab/accelsimtracing/gpu"
	"github.com/sarchlab/akita/v3/sim"
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

	d.connectionWithDevices = sim.NewDirectConnection("ConnWithDevices", b.engine, b.freq)
	d.connectionWithDevices.PlugIn(d.toDevices, 1)

	return d
}

func (b *DriverBuilder) buildPortsForDriver(d *Driver) {
	d.toDevices = sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	d.AddPort("ToDevice", d.toDevices)
}
