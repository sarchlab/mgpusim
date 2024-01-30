package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
)

type dispatcherRoundRobin struct {
}

func newDispatcherRoundRobin() *dispatcherRoundRobin {
	return &dispatcherRoundRobin{}
}

func (d *dispatcherRoundRobin) Dispatch(smunits []*smunit.SMUnit, tb *nvidia.ThreadBlock) {
	for _, warp := range tb.Warps {
		for {
			flag := false
			for _, smu := range smunits {
				if smu.IsFree() {
					smu.Execute(warp)
					flag = true
					break
				}
			}
			
			if flag {
				break
			}
		}
	}
}
