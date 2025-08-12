package trace

import (
	"bufio"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

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
	OpCodeExit
	OpCode4
	OpCode6
	OpCode8
	OpCode10
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

		filePath := "unknownopcode.log"
		appendToFileIfNotExists(filePath, rawText)
	}
	return &op
}

// Helper function to append a line to a file if it doesn't already exist
func appendToFileIfNotExists(filePath, line string) {
	// Open the file (create if not exists)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.WithField("error", err).Error("Failed to open unknownopcode.log")
		return
	}
	defer file.Close()

	// Check if the line already exists
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == line {
			return // Line already exists, skip
		}
	}

	// Append the line to the file
	_, err = file.WriteString(line + "\n")
	if err != nil {
		log.WithField("error", err).Error("Failed to write to unknownopcode.log")
	}
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

	// opcodeTable["IMAD.MOV.U32"] = Opcode{"IMAD.MOV.U32", IMADMOVU32, VariableINT32}
	opcodeTable["EXIT"] = Opcode{"EXIT", OpCodeExit, VariableDefault}

	opcodeTable["MOV"] = Opcode{"MOV", OpCodeDefault, VariableDefault}
	opcodeTable["ISETP.GE.AND"] = Opcode{"ISETP.GE.AND", OpCodeDefault, VariableDefault}

	opcodeTable["IMAD"] = Opcode{"IMAD", OpCode4, VariableDefault}
	opcodeTable["FADD"] = Opcode{"FADD", OpCode4, VariableDefault}
	opcodeTable["ULDC.64"] = Opcode{"ULDC.64", OpCode4, VariableDefault}

	opcodeTable["IMAD.WIDE"] = Opcode{"IMAD.WIDE", OpCode6, VariableDefault}

	opcodeTable["S2R"] = Opcode{"S2R", OpCode8, VariableDefault}
	opcodeTable["HFMA2.MMA"] = Opcode{"HFMA2.MMA", OpCode8, VariableDefault}

	opcodeTable["LDG.E"] = Opcode{"LDG.E", OpCodeMemRead, VariableDefault}
	opcodeTable["STG.E"] = Opcode{"STG.E", OpCodeMemWrite, VariableDefault}
}
