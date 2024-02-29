package driver

import (
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type instCountTracer struct {
	tracer *instTracer
	cu     runner.TraceableComponent
}

func (d *driver) defineMetrics() {
	d.mericsCollector = &collector{}
	d.addInstCountTracer()
}

func (d *driver) addInstCountTracer() {
	if !d.flagReportInstCount {
		return
	}

	for _, gpc := range d.gpu.GPCs {
		for _, sm := range gpc.SMs {
			for _, smu := range sm.SMUnits {
				for _, alu := range smu.ALUInt32 {
					tracer := newInstTracer{}
					d.instCountTracers = append(d.instCountTracers,
						instCountTracer{
							tracer: tracer,
							cu:     alu,
						})
					tracing.CollectTrace(cu.(tracing.NamedHookable), tracer)
				}
			}
		}
	}
}

func (d *driver) ReportStatus() {
	d.reportInstCount()
	d.dumpMetrics()
}


func (d *driver) dumpMetrics() {
	d.mericsCollector.Dump()
}