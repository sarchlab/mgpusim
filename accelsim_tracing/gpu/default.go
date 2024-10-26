package gpu

import (
	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/nvidia"
)

type defaultDispatcher struct {
	parent *GPU
}

func newDefaultDispatcher() *defaultDispatcher {
	return &defaultDispatcher{}
}

func (d *defaultDispatcher) withParent(gpu *GPU) gpuDispatcher {
	d.parent = gpu
	return d
}

func (d *defaultDispatcher) dispatch(tb *nvidia.ThreadBlock) {
	for {
		flag := false
		for _, gpc := range d.parent.gpcs {
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
