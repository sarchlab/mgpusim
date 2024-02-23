package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type dispatcherRoundRobin struct {
}

func (d *dispatcherRoundRobin) Dispatch(gpu *GPU, tb *nvidia.ThreadBlock) {
	for {
		flag := false
		for _, i := range gpu.GPCs {
			gpc := i.(*gpc.GPC)
			if gpc.IsFree() {
				gpc.Execute(tb)
				flag = true
				break
			}
		}

		if flag {
			break
		}
	}
}
