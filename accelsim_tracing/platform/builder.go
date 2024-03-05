package platform

import (
	"fmt"

	"github.com/sarchlab/accelsimtracing/driver"
	"github.com/sarchlab/accelsimtracing/gpu"
	"github.com/sarchlab/akita/v3/sim"
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
	p.Engine = sim.NewSerialEngine()
	p.Driver = new(driver.DriverBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		Build("Driver")

	gpuDriver := new(gpu.GPUBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		WithSMsCount(128).
		WithSubcoresCountPerSM(4)
	gpuCount := 1
	for i := 0; i < gpuCount; i++ {
		gpu := gpuDriver.Build(fmt.Sprintf("GPU(%d)", i))
		p.Driver.RegisterGPU(gpu)
		p.devices = append(p.devices, gpu)
	}

	return p
}

func (p *A100PlatformBuilder) freqMustBeSet() {
	if p.freq == 0 {
		panic("Frequency must be set")
	}
}
