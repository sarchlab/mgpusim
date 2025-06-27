package trace

import log "github.com/sirupsen/logrus"

type VariableType int32

const (
	VariableDefault VariableType = iota
	VariableError
	VariableINT32
	VariableFP32
	VariableFP64
)

type OpCodeType int32

const (
	OpCodeDefault OpCodeType = iota
	OpCodeError
	OpCodeMemRead
	OpCodeMemWrite
	IMADMOVU32
)

type Opcode struct {
	rawText string
	opType  OpCodeType
	varType VariableType
}

func NewOpcode(rawText string) *Opcode {
	op, ok := opcodeTable[rawText]
	if !ok {
		// op = Opcode{rawText, OpCodeError, VariableError}
		// log.WithField("opcode", rawText).Panic("Unknown opcode")
		log.WithField("opcode", rawText).Warn("Unknown opcode")
		op = Opcode{rawText, OpCodeDefault, VariableDefault}
	}
	return &op
}

func (op *Opcode) String() string {
	return op.rawText
}

func (op *Opcode) OpcodeType() OpCodeType {
	return op.opType
}

func (op *Opcode) VariableType() VariableType {
	return op.varType
}

var opcodeTable map[string]Opcode

func init() {
	opcodeTable = make(map[string]Opcode)

	opcodeTable["IMAD.MOV.U32"] = Opcode{"IMAD.MOV.U32", IMADMOVU32, VariableINT32}
	opcodeTable["MOV"] = Opcode{"MOV", OpCodeDefault, VariableDefault}
	opcodeTable["S2R"] = Opcode{"S2R", OpCodeDefault, VariableDefault}
	opcodeTable["IMAD"] = Opcode{"IMAD", OpCodeDefault, VariableDefault}
	opcodeTable["ISETP.GE.AND"] = Opcode{"ISETP.GE.AND", OpCodeDefault, VariableDefault}
	opcodeTable["EXIT"] = Opcode{"EXIT", OpCodeDefault, VariableDefault}
	opcodeTable["HFMA2.MMA"] = Opcode{"HFMA2.MMA", OpCodeDefault, VariableDefault}
	opcodeTable["ULDC.64"] = Opcode{"ULDC.64", OpCodeDefault, VariableDefault}
	opcodeTable["IMAD.WIDE"] = Opcode{"IMAD.WIDE", OpCodeDefault, VariableDefault}
	opcodeTable["LDG.E"] = Opcode{"LDG.E", OpCodeMemRead, VariableDefault}
	opcodeTable["FADD"] = Opcode{"FADD", OpCodeDefault, VariableDefault}
	opcodeTable["STG.E"] = Opcode{"STG.E", OpCodeMemWrite, VariableDefault}
}
