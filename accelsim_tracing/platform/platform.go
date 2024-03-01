package platform

import "gitlab.com/akita/akita/v3/sim"

type Platform struct {
	engine sim.Engine
	driver *Driver
	gpus   []GPU
}

func (p *Platform) Driver() *Driver {
	return p.driver
}

func (p *Platform) Engine() sim.Engine {
	return p.engine
}
