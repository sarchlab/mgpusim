package platform

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/driver"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/gpu"
)

type Platform struct {
	Engine  sim.Engine
	Driver  *driver.Driver
	Devices []*gpu.GPU
}
