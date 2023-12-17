package alu

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type int32ALU struct {
}

func newInt32ALU() *int32ALU {
	return &int32ALU{}
}

func (a *int32ALU) Execute(inst nvidia.Instruction) {

}
