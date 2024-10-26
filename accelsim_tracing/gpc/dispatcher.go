package gpc

import "github.com/sarchlab/mgpusim/v4/accelsim_tracing/nvidia"

type gpcDispatcher interface {
	withParent(gpc *GPC) gpcDispatcher
	dispatch(tb *nvidia.ThreadBlock)
}

func (g *GPC) buildDispatcher() {
	switch g.meta.gpcStrategy {
	case "default":
		g.dispatcher = newDefaultDispatcher().withParent(g)
	default:
		panic("Unknown dispatcher strategy")
	}
}
