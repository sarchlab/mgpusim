package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
)

type SMDispatcher interface {
	Dispatch([]*smunit.SMUnit, *nvidia.ThreadBlock)
}
