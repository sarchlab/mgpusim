package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type GPUDispatcher interface {
	dispatch([]*gpc.GPC, *nvidia.ThreadBlock)
}
