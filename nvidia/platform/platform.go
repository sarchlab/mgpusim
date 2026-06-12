package platform

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/gpu"
)

type Platform struct {
	Engine  sim.Engine
	Driver  *driver.Driver
	Devices []*gpu.GPU
}
