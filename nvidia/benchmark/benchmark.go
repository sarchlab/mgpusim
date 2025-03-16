package benchmark

import (
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
)

type TraceExec interface {
	ExecType() nvidiaconfig.ExecType
	Run(*driver.Driver)
}

type Benchmark struct {
	TraceExecs []TraceExec
}

type ExecMemcpy struct {
	direction nvidiaconfig.ExecMemcpyDirection
	address   uint64
	length    uint64
}

type ExecKernel struct {
	kernel nvidiaconfig.Kernel
}

func (e *ExecMemcpy) ExecType() nvidiaconfig.ExecType {
	return nvidiaconfig.ExecMemcpy
}

func (e *ExecKernel) ExecType() nvidiaconfig.ExecType {
	return nvidiaconfig.ExecKernel
}

func (e *ExecMemcpy) Run(d *driver.Driver) {
}

func (e *ExecKernel) Run(d *driver.Driver) {
	d.RunKernel(&e.kernel)
}

func (e *ExecKernel) SetKernel(kernel nvidiaconfig.Kernel) {
	e.kernel = kernel
}

func (e *ExecKernel) GetKernel() *nvidiaconfig.Kernel {
	return &e.kernel
}
