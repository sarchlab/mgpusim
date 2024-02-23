package alu

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type ALUBuilder struct {
	parentNameString string
	int32Counter     int32
}

func NewALUBuilder() *ALUBuilder {
	return &ALUBuilder{
		parentNameString: "",
		int32Counter:     0,
	}
}

func (a *ALUBuilder) WithParentNameString(parentNameString string) *ALUBuilder {
	a.parentNameString = parentNameString
	return a
}

func (a *ALUBuilder) Build(aluType nvidia.ALUType) runner.TraceableComponent {
	switch aluType {
	case "int32":
		alu := &int32ALU{
			parentNameString: a.parentNameString,
			nameID:           fmt.Sprintf("%d", a.int32Counter),
		}
		a.int32Counter++
		return alu
	default:
		panic("ALU type or number is not set")
	}
}
