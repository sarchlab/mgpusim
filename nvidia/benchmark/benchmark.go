package benchmark

import (
	"github.com/sarchlab/mgpusim/v5/nvidia/driver"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// TraceExec is one entry of a trace: a kernel launch or a memory copy.
type TraceExec interface {
	ExecType() trace.ExecType
	Run(d *driver.Driver)
}

// Benchmark is the ordered list of executions recorded in a trace directory.
type Benchmark struct {
	TraceExecs []TraceExec
}

// ExecMemcpy is a memory copy between the host and the device. Memory copies
// are not simulated.
type ExecMemcpy struct {
	Direction trace.ExecMemcpyDirection
	Address   uint64
	Length    uint64
}

// ExecType returns trace.ExecMemcpy.
func (e *ExecMemcpy) ExecType() trace.ExecType {
	return trace.ExecMemcpy
}

// Run does nothing.
func (e *ExecMemcpy) Run(_ *driver.Driver) {}

// ExecKernel is a kernel launch.
type ExecKernel struct {
	Kernel *trace.KernelTrace
}

// ExecType returns trace.ExecKernel.
func (e *ExecKernel) ExecType() trace.ExecType {
	return trace.ExecKernel
}

// Run queues the kernel on the driver.
func (e *ExecKernel) Run(d *driver.Driver) {
	d.RunKernel(e.Kernel)
}

// Load reads all the kernels and memory copies in a trace directory.
func Load(traceDirectory string) *Benchmark {
	reader := new(trace.TraceReaderBuilder).
		WithTraceDirectory(traceDirectory).
		Build()

	b := &Benchmark{}

	for _, meta := range reader.GetExecMetas() {
		switch meta.ExecType() {
		case trace.ExecKernel:
			kernel := trace.ReadTrace(meta)
			b.TraceExecs = append(b.TraceExecs, &ExecKernel{Kernel: &kernel})
		case trace.ExecMemcpy:
			b.TraceExecs = append(b.TraceExecs, &ExecMemcpy{
				Direction: meta.Direction,
				Address:   meta.Address,
				Length:    meta.Length,
			})
		default:
			panic("unknown trace entry type")
		}
	}

	return b
}
