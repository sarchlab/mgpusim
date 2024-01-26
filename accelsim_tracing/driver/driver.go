package driver

import (
	"errors"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type Driver struct {
	benchmark *Benchmark
	gpu       *gpu.GPU
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
