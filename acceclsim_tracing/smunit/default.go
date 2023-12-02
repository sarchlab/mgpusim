package smunit

import "github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"

type defaultDispatcher struct {
	parent *SMUnit
}

func newDefaultDispatcher() *defaultDispatcher {
	return &defaultDispatcher{}
}

func (d *defaultDispatcher) withParent(sm *SMUnit) smUnitDispatcher {
	d.parent = sm
	return d
}

func (d *defaultDispatcher) dispatch(warp *nvidia.Warp) {
}
