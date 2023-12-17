package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type dispatcherRoundRobin struct {
}

func newDispatcherRoundRobin() *dispatcherRoundRobin {
	return &dispatcherRoundRobin{}
}

func (d *dispatcherRoundRobin) Dispatch(gpcs []*gpc.GPC, tb *nvidia.ThreadBlock) {
	for {
		flag := false
		for _, gpc := range gpcs {
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
