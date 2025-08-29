package platform

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

type A100PlatformBuilder struct {
	freq sim.Freq
}

func (b *A100PlatformBuilder) WithFreq(freq sim.Freq) *A100PlatformBuilder {
	b.freq = freq
	return b
}

func (b *A100PlatformBuilder) Build() *Platform {
	b.freqMustBeSet()

	p := new(Platform)
	p.Simulation = simulation.MakeBuilder().Build()
	p.Engine = p.Simulation.GetEngine()
	
	p.Driver = new(driver.DriverBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		Build("Driver")

	// Register Driver with simulation
	p.Simulation.RegisterComponent(p.Driver)

	gpuDriver := new(gpu.GPUBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		WithSMsCount(108).
		WithSubcoresCountPerSM(4)
	gpuCount := 1
	for i := 0; i < gpuCount; i++ {
		gpu := gpuDriver.Build(fmt.Sprintf("GPU(%d)", i))
		p.Driver.RegisterGPU(gpu)
		p.Devices = append(p.Devices, gpu)
		
		// Register GPU with simulation
		p.Simulation.RegisterComponent(gpu)
	}

	// Enable tracing for all components
	visTracer := p.Simulation.GetVisTracer()
	for _, comp := range p.Simulation.Components() {
		tracing.CollectTrace(comp.(tracing.NamedHookable), visTracer)
	}

	return p
}

func (b *A100PlatformBuilder) freqMustBeSet() {
	if b.freq == 0 {
		log.Panic("Frequency must be set")
	}
}
