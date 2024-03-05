package platform

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/driver"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type Platform struct {
	Engine  sim.Engine
	Driver  *driver.Driver
	Devices []*gpu.GPU
}
