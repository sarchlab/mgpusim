package platform

import (
	"fmt"

	"gitlab.com/akita/akita/v3/sim"
)

type PlatformBuilder struct {
	gpuCount int16
	smPerGPU int16
	freq     sim.Freq
}

func (b *PlatformBuilder) WithGPUCount(count int16) *PlatformBuilder {
	b.gpuCount = count
	return b
}

func (b *PlatformBuilder) WithSMPerGPU(smPerGPU int16) *PlatformBuilder {
	b.smPerGPU = smPerGPU
	return b
}

func (b *PlatformBuilder) WithFreq(freq sim.Freq) *PlatformBuilder {
	b.freq = freq
	return b
}

func (b *PlatformBuilder) Build() *Platform {
	b.gpuFreqMustBeSet()
	b.freqMustBeSet()

	p := new(Platform)
	p.engine = sim.NewSerialEngine()
	p.driver = NewDriver("Driver", p.engine, b.freq)

	for i := int16(0); i < b.gpuCount; i++ {
		gpuName := fmt.Sprintf("GPU%d", i)
		p.gpu = NewGPU(gpuName, p.engine, b.freq, b.smPerCount)
	}

	p.driver.RegisterGPU(p.gpu)

	return p
}

func (p *PlatformBuilder) gpuFreqMustBeSet() {
	if p.freq == 0 {
		panic("GPU frequency must be set")
	}
}

func (p *PlatformBuilder) freqMustBeSet() {
	if p.freq == 0 {
		panic("Frequency must be set")
	}
}
