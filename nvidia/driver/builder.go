package driver

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

type DriverBuilder struct {
	engine                    sim.Engine
	freq                      sim.Freq
	driver2GPUOverheadLatency uint64
}

func (b *DriverBuilder) WithEngine(engine sim.Engine) *DriverBuilder {
	b.engine = engine
	return b
}

func (b *DriverBuilder) WithFreq(freq sim.Freq) *DriverBuilder {
	b.freq = freq
	return b
}

func (b *DriverBuilder) WithDriver2GPUOverheadLatency(latency uint64) *DriverBuilder {
	b.driver2GPUOverheadLatency = latency
	return b
}

func (b *DriverBuilder) Build(name string) *Driver {
	d := &Driver{
		devices:                            make(map[string]*gpu.GPUController),
		driver2GPUOverheadLatency:          b.driver2GPUOverheadLatency,
		driver2GPUOverheadLatencyRemaining: b.driver2GPUOverheadLatency,
	}

	d.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, d)
	b.buildPortsForDriver(d)

	d.connectionWithDevices = directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		Build("ConnWithDevices")
	d.connectionWithDevices.PlugIn(d.toDevices)

	return d
}

func (b *DriverBuilder) buildPortsForDriver(d *Driver) {
	d.toDevices = sim.NewPort(d, 4096, 4096, "ToDevice")
	d.AddPort("ToDevice", d.toDevices)
}
