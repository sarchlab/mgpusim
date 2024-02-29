package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type GPUDispatcher interface {
	Dispatch(*GPU, *nvidia.ThreadBlock)
}

func (g *GPUBuilder) buildDispatcher() GPUDispatcher {
	switch g.gpuDispatchStrategy {
	case "round-robin":
		return &dispatcherRoundRobin{}
	default:
		panic("GPU strategy is not supported")
	}
}
