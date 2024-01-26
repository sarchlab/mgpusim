package driver

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"

type DriverBuilder struct {
	benchmark *Benchmark
	gpu       *gpu.GPU
}

func NewDriverBuilder() *DriverBuilder {
	return &DriverBuilder{
		benchmark: nil,
		gpu:       nil,
	}
}

func (d *DriverBuilder) WithBenchmark(b *Benchmark) *DriverBuilder {
	d.benchmark = b
	return d
}

func (d *DriverBuilder) WithGPU(g *gpu.GPU) *DriverBuilder {
	d.gpu = g
	return d
}

func (d *DriverBuilder) Build() (*Driver, error) {
	return &Driver{
		benchmark: d.benchmark,
		gpu:       d.gpu,
	}, nil
}
