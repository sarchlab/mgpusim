package gpu

import "github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"

type gpuDispatcher interface {
	withParent(gpu *GPU) gpuDispatcher
	dispatch(tb *nvidia.ThreadBlock)
}

func (g *GPU) buildDispatcher() {
	switch g.meta.gpuStrategy {
	case "default":
		g.dispatcher = newDefaultDispatcher().withParent(g)
	default:
		panic("Unknown dispatcher strategy")
	}
}
