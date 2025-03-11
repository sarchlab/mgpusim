package benchmark

import (
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/driver"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
)

type TraceExec interface {
	ExecType() nvidia.ExecType
	Run(*driver.Driver)
}

type Benchmark struct {
	TraceExecs []TraceExec
}

type ExecMemcpy struct {
	direction nvidia.ExecMemcpyDirection
	address   uint64
	length    uint64
}

type ExecKernel struct {
	kernel nvidia.Kernel
}

func (e *ExecMemcpy) ExecType() nvidia.ExecType {
	return nvidia.ExecMemcpy
}

func (e *ExecKernel) ExecType() nvidia.ExecType {
	return nvidia.ExecKernel
}

func (e *ExecMemcpy) Run(d *driver.Driver) {
}

func (e *ExecKernel) Run(d *driver.Driver) {
	d.RunKernel(&e.kernel)
}

func (e *ExecKernel) SetKernel(kernel nvidia.Kernel) {
	e.kernel = kernel
}

func (e *ExecKernel) GetKernel() *nvidia.Kernel {
	return &e.kernel
}
