package driver

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"

type DriverBuilder struct {
	gpu *gpu.GPU
}

func NewDriverBuilder() *DriverBuilder {
	return &DriverBuilder{
		gpu: nil,
	}
}
func (d *DriverBuilder) WithGPU(g *gpu.GPU) *DriverBuilder {
	d.gpu = g
	return d
}

func (d *DriverBuilder) Build() (*driver, error) {
	return &driver{
		gpu: d.gpu,
	}, nil
}
