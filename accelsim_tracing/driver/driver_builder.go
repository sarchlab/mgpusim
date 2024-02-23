package driver

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"

type DriverBuilder struct {
	gpu                 *gpu.GPU
	flagReportInstCount bool
}

func NewDriverBuilder() *DriverBuilder {
	return &DriverBuilder{
		gpu:                 nil,
		flagReportInstCount: false,
	}
}

func (d *DriverBuilder) WithGPU(g *gpu.GPU) *DriverBuilder {
	d.gpu = g
	return d
}

func (d *DriverBuilder) WithInstCountReport() *DriverBuilder {
	d.flagReportInstCount = true
	return d
}

func (d *DriverBuilder) Build() (*driver, error) {
	dv := &driver{
		gpu: d.gpu,
		flagReportInstCount: d.flagReportInstCount,
	}
	dv.addInstCountTracer()

	return dv, nil
}
