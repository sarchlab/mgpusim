package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type SMDispatcher interface {
	Dispatch(*SM, *nvidia.ThreadBlock)
}

func (s *SMBuiler) buildDispatcher() SMDispatcher {
	switch s.smDispatchStrategy {
	case "round-robin":
		return &dispatcherRoundRobin{}
	default:
		panic("Unknown dispatch strategy")
	}
}
