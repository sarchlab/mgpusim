package component

import (
	"github.com/sarchlab/akita/v3/sim"
)

type Platform struct {
	engine sim.Engine
	driver *Driver
	gpu    *GPU
}

func NewTickingPlatform() *Platform {
	p := &Platform{
		engine: sim.NewSerialEngine(),
	}
	p.driver = NewDriver("Driver", p.engine, 1*sim.Hz)
	p.gpu = NewGPU("GPU", p.engine, 1*sim.Hz, 16)
	p.driver.RegisterGPU(p.gpu)

	return p
}

func (p *Platform) Driver() *Driver {
	return p.driver
}

func (p *Platform) Engine() sim.Engine {
	return p.engine
}
