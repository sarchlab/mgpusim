package benchmark

import "github.com/sarchlab/accelsimtracing/nvidia"

type Benchmark struct {
	traceExecs []TraceExec
}

func (b *Benchmark) TraceExecs() []TraceExec {
	return b.traceExecs
}

type TraceExec interface {
	ExecType() nvidia.ExecType
	Run()
}

type ExecMemcpy struct {
	direction nvidia.ExecMemcpyDirection
	address   uint64
	length    uint64
}

type ExecKernel struct {
	kernel Kernel
}

func (e *ExecMemcpy) ExecType() nvidia.ExecType {
	return nvidia.ExecMemcpy
}

func (e *ExecKernel) ExecType() nvidia.ExecType {
	return nvidia.ExecKernel
}

func (e *ExecMemcpy) Run() {
}

func (e *ExecKernel) Run() {
}
