package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
)

type dispatcherRoundRobin struct {
}

func (d *dispatcherRoundRobin) Dispatch(sm *SM, tb *nvidia.ThreadBlock) {
	for _, warp := range tb.Warps {
		for {
			flag := false
			for _, i := range sm.SMUnits {
				smu := i.(*smunit.SMUnit)
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
