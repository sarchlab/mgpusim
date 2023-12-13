package smunit

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type smUnitDispatcher interface {
	withParent(sm *SMUnit) smUnitDispatcher
	dispatch(tb *nvidia.Warp)
}

func (s *SMUnit) buildDispatcher() {
	switch s.meta.smUnitStrategy {
	case "default":
		s.dispatcher = newDefaultDispatcher().withParent(s)
	default:
		panic("Unknown dispatch strategy")
	}
}
