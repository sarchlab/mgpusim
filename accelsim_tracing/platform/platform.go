package platform

import (
	"github.com/sarchlab/accelsimtracing/driver"
	"github.com/sarchlab/accelsimtracing/gpu"
	"github.com/sarchlab/akita/v3/sim"
)

type Platform struct {
	Engine  sim.Engine
	Driver  *driver.Driver
	Devices []*gpu.GPU
}
