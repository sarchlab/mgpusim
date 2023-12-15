package alu

type ALUGroupBuilder struct {
	aluType string
	aluNum  int32
}

func NewALUGroupBuilder() *ALUGroupBuilder {
	return &ALUGroupBuilder{
		aluType: "",
		aluNum:  0,
	}
}

func (a *ALUGroupBuilder) WithALUType(aluType string) *ALUGroupBuilder {
	a.aluType = aluType
	return a
}

func (a *ALUGroupBuilder) WithALUNum(num int32) *ALUGroupBuilder {
	a.aluNum = num
	return a
}

func (a *ALUGroupBuilder) Build() *ALUGroup {
	if a.aluType == "" || a.aluNum == 0 {
		panic("ALU type or number is not set")
	}
	ag := new(ALUGroup)
	ag.alus = make([]aluInterface, a.aluNum)
	for i := range ag.alus {
		ag.alus[i] = a.buildALU(a.aluType)
	}
	return ag
}

func (a *ALUGroupBuilder) buildALU(aluT string) aluInterface {
	switch aluT {
	case "int32":
		return newInt32ALU()
	default:
		panic("Unknown ALU type")
	}
}
