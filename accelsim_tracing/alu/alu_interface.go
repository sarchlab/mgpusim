package alu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
	"gitlab.com/akita/akita/v3/sim"
)

type ALU interface {
	sim.TickingComponent
	runner.TraceableComponent

	Execute(inst nvidia.Instruction)
}
