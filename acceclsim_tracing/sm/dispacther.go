package sm

import "github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"

type smDispatcher interface {
	withParent(sm *SM) smDispatcher
	dispatch(tb *nvidia.ThreadBlock)
}

func (s *SM) buildDispatcher() {
	switch s.meta.smStrategy {
	case "default":
		s.dispatcher = newDefaultDispatcher().withParent(s)
	default:
		panic("Unknown dispatch strategy")
	}
}
