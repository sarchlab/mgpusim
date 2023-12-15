package alu

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type AluGroup struct {
	alus []aluInterface
}

func (a *AluGroup) Execute(inst nvidia.Instruction) {
	for _, alu := range a.alus {
		alu.Execute(inst)
	}
}
