package disasm

// An Operand is an operand
type Operand struct {
	Name  string
	IsReg bool
	IsNum bool
	Value float64
}

// An Instruction is a GCN3 instructino
type Instruction struct {
	*Format
	*InstType
	ByteSize int

	SSRC0 Operand
	SSRC1 Operand
	SDST  Operand
}

func (i Instruction) String() string {
	return i.InstName
}
