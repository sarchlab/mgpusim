package gpc

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/sm"
)

type GPC struct {
	sms []*sm.SM
}

func (g *GPC) IsFree() bool {
	return true
}

func (g *GPC) Execute(tb *nvidia.ThreadBlock) {
	for _, sm := range g.sms {
		sm.Execute(tb)
	}
}
