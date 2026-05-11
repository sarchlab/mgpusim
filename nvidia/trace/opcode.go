package trace

import (
	"bufio"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// type VariableType int32

// const (
// 	VariableDefault VariableType = iota
// 	VariableError
// 	VariableINT32
// 	VariableFP32
// 	VariableFP64
// )

// type OpCodeType int32

// const (
// 	OpCodeDefault OpCodeType = iota
// 	OpCodeError
// 	OpCodeMemRead
// 	OpCodeMemWrite
// 	OpCodeExit
// 	OpCode2
// 	OpCode3
// 	OpCode4
// 	OpCode5
// 	OpCode6
// 	OpCode8
// 	OpCode10
// 	OpCode20
// 	OpCode40
// 	OpCode100
// 	IMADMOVU32
// )

type Opcode struct {
	rawText string
}

// opType  OpCodeType
// varType VariableType

func NewOpcode(rawText string) *Opcode {
	op, ok := opcodeTable[rawText]
	if !ok {
		// op = Opcode{rawText, OpCodeError, VariableError}
		// log.WithField("opcode", rawText).Panic("Unknown opcode")
		log.WithField("opcode", rawText).Warn("Unknown opcode")
		// op = Opcode{rawText}
		op = Opcode{rawText}

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

// func (op *Opcode) OpcodeType() OpCodeType {
// 	return op.opType
// }

// func (op *Opcode) VariableType() VariableType {
// 	return op.varType
// }

// func (op *Opcode) GetInstructionCycles() uint64 {
// 	offset := uint64(0)
// 	cycle := uint64(0)
// 	switch op.opType {
// 	case OpCodeDefault:
// 		cycle = 1
// 	case OpCode2:
// 		cycle = 2
// 	case OpCode3:
// 		cycle = 3
// 	case OpCode4:
// 		cycle = 4
// 	case OpCode5:
// 		cycle = 5
// 	case OpCode6:
// 		cycle = 6
// 	case OpCode8:
// 		cycle = 8
// 	case OpCode10:
// 		cycle = 10
// 	case OpCode20:
// 		cycle = 20
// 	case OpCode40:
// 		cycle = 40
// 	case OpCode100:
// 		cycle = 100
// 	case OpCodeExit:
// 		cycle = 1
// 	case OpCodeMemRead:
// 		cycle = 0
// 	case OpCodeMemWrite:
// 		cycle = 0
// 	}
// 	return max(cycle-offset, 1)
// }

var opcodeTable map[string]Opcode

func init() {
	opcodeTable = make(map[string]Opcode)

	opcodeTable["EXIT"] = Opcode{"EXIT"}

	// OpCodeMemRead
	opcodeTable["LDG.E"] = Opcode{"LDG.E"}

	// OpCodeMemWrite
	opcodeTable["STG.E"] = Opcode{"STG.E"}

	// OpCodeDefault (1)
	opcodeTable["MOV"] = Opcode{"MOV"}
	opcodeTable["ISETP.GE.AND"] = Opcode{"ISETP.GE.AND"}

	// OpCode2
	opcodeTable["UMOV"] = Opcode{"UMOV"}

	// OpCode3
	opcodeTable["LEA"] = Opcode{"LEA"}
	opcodeTable["S2R"] = Opcode{"S2R"}

	// OpCode4
	opcodeTable["ULDC"] = Opcode{"ULDC"}
	opcodeTable["ULDC.64"] = Opcode{"ULDC.64"}
	opcodeTable["LDC"] = Opcode{"LDC"} // OpCode4
	opcodeTable["SHF.R.S32.HI"] = Opcode{"SHF.R.S32.HI"}
	opcodeTable["ISETP.GE.U32.AND"] = Opcode{"ISETP.GE.U32.AND"}
	opcodeTable["IMAD.MOV.U32"] = Opcode{"IMAD.MOV.U32"}
	opcodeTable["ISETP.GT.AND"] = Opcode{"ISETP.GT.AND"}
	opcodeTable["FFMA"] = Opcode{"FFMA"} // OpCode4
	opcodeTable["VIADD"] = Opcode{"VIADD"}
	opcodeTable["ISETP.NE.OR"] = Opcode{"ISETP.NE.OR"}
	opcodeTable["ISETP.NE.AND"] = Opcode{"ISETP.NE.AND"}
	opcodeTable["S2UR"] = Opcode{"S2UR"}
	opcodeTable["IMAD.U32"] = Opcode{"IMAD.U32"}
	opcodeTable["ISETP.LT.U32.AND"] = Opcode{"ISETP.LT.U32.AND"}
	opcodeTable["ISETP.NE.U32.AND"] = Opcode{"ISETP.NE.U32.AND"}
	opcodeTable["IMAD.MOV"] = Opcode{"IMAD.MOV"}
	opcodeTable["SHF.L.U32"] = Opcode{"SHF.L.U32"}
	opcodeTable["SHF.R.U32.HI"] = Opcode{"SHF.R.U32.HI"}
	opcodeTable["FMUL"] = Opcode{"FMUL"}
	opcodeTable["IMAD"] = Opcode{"IMAD"}
	opcodeTable["FADD"] = Opcode{"FADD"} // OpCode4

	// OpCode5 (rounded mean for ranges)
	opcodeTable["ISETP.GT.U32.AND.EX"] = Opcode{"ISETP.GT.U32.AND.EX"}
	opcodeTable["LEA.HI.X.SX32"] = Opcode{"LEA.HI.X.SX32"}

	// OpCode6
	opcodeTable["BRA"] = Opcode{"BRA"}     // OpCode6
	opcodeTable["IADD3"] = Opcode{"IADD3"} // OpCode6
	opcodeTable["LOP3.LUT"] = Opcode{"LOP3.LUT"}
	opcodeTable["IMAD.WIDE.U32"] = Opcode{"IMAD.WIDE.U32"}
	opcodeTable["IMAD.IADD"] = Opcode{"IMAD.IADD"}
	opcodeTable["IMAD.WIDE"] = Opcode{"IMAD.WIDE"} // OpCode6
	opcodeTable["LEA.HI.X"] = Opcode{"LEA.HI.X"}
	opcodeTable["IADD3.X"] = Opcode{"IADD3.X"}
	opcodeTable["PLOP3.LUT"] = Opcode{"PLOP3.LUT"}
	opcodeTable["UIADD3"] = Opcode{"UIADD3"}
	opcodeTable["UIMAD.WIDE"] = Opcode{"UIMAD.WIDE"}
	opcodeTable["I2F.U32.RP"] = Opcode{"I2F.U32.RP"}
	opcodeTable["IMAD.HI.U32"] = Opcode{"IMAD.HI.U32"}
	opcodeTable["SHF.R.S64"] = Opcode{"SHF.R.S64"}

	// OpCode8
	opcodeTable["F2I.FTZ.U32.TRUNC.NTZ"] = Opcode{"F2I.FTZ.U32.TRUNC.NTZ"}

	// OpCode10
	opcodeTable["HFMA2.MMA"] = Opcode{"HFMA2.MMA"}

	// OpCode40
	opcodeTable["MUFU.RCP"] = Opcode{"MUFU.RCP"}
}
