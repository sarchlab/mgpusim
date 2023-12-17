package alu

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

type ALU interface {
	Execute(inst nvidia.Instruction)
}
