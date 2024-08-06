package sm

import "github.com/sarchlab/mgpusim/v4/accelsim_tracing/nvidia"

type defaultDispatcher struct {
	parent *SM
}

func newDefaultDispatcher() *defaultDispatcher {
	return &defaultDispatcher{}
}

func (d *defaultDispatcher) withParent(sm *SM) smDispatcher {
	d.parent = sm
	return d
}

func (d *defaultDispatcher) dispatch(tb *nvidia.ThreadBlock) {
	for _, warp := range tb.Warps {
		for {
			flag := false
			for _, smUnit := range d.parent.smUnits {
				if smUnit.IsFree() {
					smUnit.Execute(warp)
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
