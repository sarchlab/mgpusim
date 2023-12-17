package alu

type ALUBuilder struct {
	aluType string
}

func NewALUBuilder() *ALUBuilder {
	return &ALUBuilder{
		aluType: "",
	}
}

func (a *ALUBuilder) WithALUType(aluType string) *ALUBuilder {
	a.aluType = aluType
	return a
}

func (a *ALUBuilder) Build() ALU {
	switch a.aluType {
	case "int32":
		return newInt32ALU()
	default:
		panic("ALU type or number is not set")
	}
}
