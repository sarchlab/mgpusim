package alu

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type ALU interface {
	withParent(aluGroup *ALUGroup) ALU
	Execute(inst nvidia.Instruction)
}

func (a *ALUGroup) newALU() ALU {
	switch a.meta.aluType {
	case "int32":
		return newInt32ALU().withParent(a)
	default:
		panic("Unknown ALU type")
	}
}
