package gpc

import "github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"

type defaultDispatcher struct {
	parent *GPC
}

func newDefaultDispatcher() *defaultDispatcher {
	return &defaultDispatcher{}
}

func (d *defaultDispatcher) withParent(gpc *GPC) gpcDispatcher {
	d.parent = gpc
	return d
}

func (d *defaultDispatcher) dispatch(tb *nvidia.ThreadBlock) {
	for {
		flag := false
		for _, sm := range d.parent.sms {
			if sm.IsFree() {
				sm.Execute(tb)
				flag = true
				break
			}
		}
		if flag {
			break
		}
	}
}
