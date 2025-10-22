package platform

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

// H100PlatformBuilder builds a platform with NVIDIA H100 GPU configuration
type H100PlatformBuilder struct {
	freq sim.Freq
}

// WithFreq sets the frequency for the platform
func (b *H100PlatformBuilder) WithFreq(freq sim.Freq) *H100PlatformBuilder {
	b.freq = freq
	return b
}

// Build creates a platform with H100 GPU configuration
func (b *H100PlatformBuilder) Build() *Platform {
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
		WithSMsCount(132).          // H100 has 132 SMs
		WithSubcoresCountPerSM(4)   // Each SM has 4 subcores
	
	gpuCount := 1
	for i := 0; i < gpuCount; i++ {
		gpu := gpuDriver.Build(fmt.Sprintf("GPU(%d)", i))
		p.Driver.RegisterGPU(gpu)
		p.Devices = append(p.Devices, gpu)
	}

	return p
}

func (b *H100PlatformBuilder) freqMustBeSet() {
	if b.freq == 0 {
		log.Panic("Frequency must be set")
	}
}
