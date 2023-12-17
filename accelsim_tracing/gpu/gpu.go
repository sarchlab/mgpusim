package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type GPU struct {
	dispatcher GPUDispatcher
	gpcs       []*gpc.GPC
}

// RunThreadBlock runs a threadblock on the GPU
// [todo] how to handle the relationship between trace.threadblock and truethreadblock
func (g *GPU) RunThreadBlock(tb *nvidia.ThreadBlock) {
	g.dispatcher.Dispatch(g.gpcs, tb)
}
