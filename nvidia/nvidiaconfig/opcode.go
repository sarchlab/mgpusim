package nvidiaconfig

import log "github.com/sirupsen/logrus"

// VariableType [todo] how to construct these?
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
		op = Opcode{rawText, OpCodeError, VariableError}
		log.WithField("opcode", rawText).Panic("Unknown opcode")
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
}
