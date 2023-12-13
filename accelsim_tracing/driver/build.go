package driver

import (
	"errors"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type Driver struct {
	benchmark *Benchmark
	gpu       *gpu.GPU
}

func NewDriver() *Driver {
	return &Driver{
		benchmark: nil,
		gpu:       nil,
	}
}

func (d *Driver) WithBenchmark(b *Benchmark) *Driver {
	d.benchmark = b
	return d
}

func (d *Driver) WithGPU(g *gpu.GPU) *Driver {
	d.gpu = g
	return d
}

func (d *Driver) Build() error {
	return nil
}

func (d *Driver) Exec() error {
	if d.benchmark == nil {
		return errors.New("no trace parser specified")
	} else if d.gpu == nil {
		return errors.New("no gpu specified")
	}

	for _, e := range *d.benchmark.TraceExecs {
		err := e.Exec(d.gpu)
		if err != nil {
			return err
		}
	}
	return nil
}
