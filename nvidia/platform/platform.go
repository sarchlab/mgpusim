package platform

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

type Platform struct {
	Engine     sim.Engine
	Simulation *simulation.Simulation
	Driver     *driver.Driver
	Devices    []*gpu.GPU
}
