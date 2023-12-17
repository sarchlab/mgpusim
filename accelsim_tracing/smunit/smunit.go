package smunit

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/alu"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type SMUnit struct {
	registerFile *RegisterFile
	aluInt32     []alu.ALU
}

func (s *SMUnit) Execute(warp *nvidia.Warp) {

}

func (s *SMUnit) IsFree() bool {
	return true
}
