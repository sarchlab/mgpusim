package platform

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

type A100PlatformBuilder struct {
	freq       sim.Freq
	simulation *simulation.Simulation
	VisTracing bool
}

func (b *A100PlatformBuilder) WithFreq(freq sim.Freq) *A100PlatformBuilder {
	b.freq = freq
	return b
}

// WithSimulation sets the simulation to use.
func (b *A100PlatformBuilder) WithSimulation(sim *simulation.Simulation) *A100PlatformBuilder {
	b.simulation = sim
	return b
}

func (b *A100PlatformBuilder) WithVisTracing(vt bool) *A100PlatformBuilder {
	b.VisTracing = vt
	return b
}

func (b *A100PlatformBuilder) Build() *Platform {
	b.freqMustBeSet()

	p := new(Platform)
	// p.Engine = sim.NewSerialEngine()
	p.Engine = b.simulation.GetEngine()
	p.Driver = new(driver.DriverBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		Build("Driver")

	gpuDriver := new(gpu.GPUBuilder).
		WithEngine(p.Engine).
		WithFreq(b.freq).
		WithSimulation(b.simulation).
		WithSMsCount(108).
		WithSMSPsCountPerSM(4).
		WithL2CacheSize(40 * mem.MB). // WithL2CacheSize(2 * mem.MB).
		WithDRAMSize(80 * mem.GB).    // WithDRAMSize(4 * mem.GB).
		WithLog2CacheLineSize(9).     // WithLog2CacheLineSize(6). Changed from 7 to 9 to support 512-byte memory accesses
		WithNumMemoryBank(4).
		WithVisTracing(b.VisTracing)
	gpuCount := 1
	for i := 0; i < gpuCount; i++ {
		gpu := gpuDriver.Build(fmt.Sprintf("GPU[%d]", i))
		p.Driver.RegisterGPU(gpu)
		p.Devices = append(p.Devices, gpu)
	}

	return p
}

func (b *A100PlatformBuilder) freqMustBeSet() {
	if b.freq == 0 {
		log.Panic("Frequency must be set")
	}
}
