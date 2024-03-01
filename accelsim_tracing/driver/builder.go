package driver

import (
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
	d := &Driver{}
	d.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, d)
	d.toDevices = sim.NewLimitNumMsgPort(d, 4, "ToDevice")
	d.connectionWithDevices = sim.NewDirectConnection("ConnWithDevices", b.engine, b.freq)
	d.connectionWithDevices.PlugIn(d.toDevices, 1)
	return d
}
