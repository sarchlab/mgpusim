package alu

import "github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"

type ALUGroup struct {
	meta *aluGroupMetaData
	alus []ALU
}

type aluGroupMetaData struct {
	aluType string
	aluNum  int32
}

func NewALUGroup() *ALUGroup {
	return &ALUGroup{
		meta: &aluGroupMetaData{
			aluType: "undefined",
			aluNum:  0,
		},
	}
}

func (a *ALUGroup) WithALUType(aluType string) *ALUGroup {
	a.meta.aluType = aluType
	return a
}

func (a *ALUGroup) WithALUNum(num int32) *ALUGroup {
	a.meta.aluNum = num
	return a
}

func (a *ALUGroup) Build() {
	a.alus = make([]ALU, a.meta.aluNum)
	for i := range a.alus {
		a.alus[i] = a.newALU()
	}
}

func (a *ALUGroup) Execute(inst nvidia.Instruction) {
	for _, alu := range a.alus {
		alu.Execute(inst)
	}
}
