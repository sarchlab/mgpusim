package alu

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type aluInterface interface {
	Execute(inst nvidia.Instruction)
}
