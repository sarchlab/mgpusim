package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type GPUDispatcher interface {
	Dispatch([]*gpc.GPC, *nvidia.ThreadBlock)
}
