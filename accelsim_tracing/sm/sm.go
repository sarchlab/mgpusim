package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
)

type SM struct {
	smUnits    []*smunit.SMUnit
	dispatcher SMDispatcher
}

func (s *SM) Execute(tb *nvidia.ThreadBlock) {
	s.dispatcher.Dispatch(s.smUnits, tb)
}
