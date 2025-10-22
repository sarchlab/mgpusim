package benchmark

import (
	"github.com/sarchlab/mgpusim/v4/nvidia/driver"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type TraceExec interface {
	ExecType() trace.ExecType
	Run(*driver.Driver)
}

type Benchmark struct {
	TraceExecs []TraceExec
}

type ExecMemcpy struct {
	direction trace.ExecMemcpyDirection
	address   uint64
	length    uint64
}

type ExecKernel struct {
	kernel trace.KernelTrace
}

func (e *ExecMemcpy) ExecType() trace.ExecType {
	return trace.ExecMemcpy
}

func (e *ExecKernel) ExecType() trace.ExecType {
	return trace.ExecKernel
}

func (e *ExecMemcpy) Run(d *driver.Driver) {
}

func (e *ExecKernel) Run(d *driver.Driver) {
	d.RunKernel(&e.kernel)
}

func (e *ExecKernel) SetKernel(kernel trace.KernelTrace) {
	e.kernel = kernel
}

func (e *ExecKernel) GetKernel() *trace.KernelTrace {
	return &e.kernel
}
