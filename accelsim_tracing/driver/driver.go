package driver

import (
	"errors"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
	trace "github.com/sarchlab/mgpusim/v3/accelsim_tracing/trace"
)

type Driver struct {
	gpu *gpu.GPU
}

func (d *Driver) Exec(bm *Benchmark) error {
	if bm == nil {
		return errors.New("no trace parser specified")
	}
	if d.gpu == nil {
		return errors.New("no gpu specified")
	}

	for _, e := range *bm.TraceExecs {
		if e.Type() == "memcopy" {
			// [todo] implement
		} else if e.Type() == "kernel" {
			builder := trace.NewTraceGroupReaderBuilder().WithFilePath(e.File())
			tgReader := builder.Build()

			tgReader.ParseThreadBlocks()
			for it := tgReader.ThreadBlockQueue.Front(); it != nil; it = it.Next() {
				d.gpu.RunThreadBlock(it.Value.(*trace.ThreadBlock).GenerateNVThreadBlock())
			}

			tgReader.File.Close()
		}
	}
	
	return nil
}
