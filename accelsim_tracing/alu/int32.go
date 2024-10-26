package alu

import "github.com/sarchlab/mgpusim/v4/accelsim_tracing/nvidia"

type int32ALU struct {
	parent *ALUGroup
}

func newInt32ALU() *int32ALU {
	return &int32ALU{}
}

func (a *int32ALU) withParent(aluGroup *ALUGroup) ALU {
	a.parent = aluGroup
	return a
}

func (a *int32ALU) Execute(inst nvidia.Instruction) {
}
