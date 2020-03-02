package insts

import (
	"debug/elf"
	"fmt"
	"log"
	"strings"
)

// ExeUnit defines which execution unit should execute the instruction
type ExeUnit int

// Defines all possible execution units
const (
	ExeUnitVALU ExeUnit = iota
	ExeUnitScalar
	ExeUnitVMem
	ExeUnitBranch
	ExeUnitLDS
	ExeUnitGDS
	ExeUnitSpecial
)

// A InstType represents an instruction type. For example s_barrier instruction
// is a instruction type
type InstType struct {
	InstName  string
	Opcode    Opcode
	Format    *Format
	ID        int
	ExeUnit   ExeUnit
	DSTWidth  int
	SRC0Width int
	SRC1Width int
	SRC2Width int
	SDSTWidth int
}

// An Inst is a GCN3 instruction
type Inst struct {
	*Format
	*InstType
	ByteSize int
	PC       uint64

	Src0 *Operand
	Src1 *Operand
	Src2 *Operand
	Dst  *Operand
	SDst *Operand // For VOP3b

	Addr   *Operand
	Data   *Operand
	Data1  *Operand
	Base   *Operand
	Offset *Operand
	SImm16 *Operand

	Abs                 int
	Omod                int
	Neg                 int
	Offset0             uint8
	Offset1             uint8
	SystemLevelCoherent bool
	GlobalLevelCoherent bool
	TextureFailEnable   bool
	Imm                 bool
	Clamp               bool
	GDS                 bool
	VMCNT               int
	LKGMCNT             int

	//Fields for SDWA extensions
	IsSdwa    bool
	DstSel    SDWASelect
	DstUnused SDWAUnused
	Src0Sel   SDWASelect
	Src0Sext  bool
	Src0Neg   bool
	Src0Abs   bool
	Src1Sel   SDWASelect
	Src1Sext  bool
	Src1Neg   bool
	Src1Abs   bool
}

// NewInst creates a zero-filled instruction
func NewInst() *Inst {
	i := new(Inst)
	i.Format = new(Format)
	i.InstType = new(InstType)
	return i
}

func (i Inst) sop2String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String() + ", " +
		i.Src1.String()
}

func (i Inst) vop1String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String()
}

func (i Inst) flatString() string {
	var s string
	if i.Opcode >= 16 && i.Opcode <= 23 {
		s = i.InstName + " " + i.Dst.String() + ", " +
			i.Addr.String()
	} else if i.Opcode >= 24 && i.Opcode <= 31 {
		s = i.InstName + " " + i.Addr.String() + ", " +
			i.Data.String()
	}
	return s
}

func (i Inst) smemString() string {
	// TODO: Consider store instructions, and the case if imm = 0
	s := fmt.Sprintf("%s %s, %s, %#x",
		i.InstName, i.Data.String(), i.Base.String(), uint16(i.Offset.IntValue))
	return s
}

func (i Inst) soppString(file *elf.File) string {
	operandStr := ""
	if i.Opcode == 12 { // S_WAITCNT
		operandStr = i.waitcntOperandString()
	} else if i.Opcode >= 2 && i.Opcode <= 9 { // Branch
		symbolFound := false
		if file != nil {
			imm := int16(uint16(i.SImm16.IntValue))
			target := i.PC + uint64(imm*4) + 4
			symbols, _ := file.Symbols()
			for _, symbol := range symbols {
				if symbol.Value == target {
					operandStr = " " + symbol.Name
					symbolFound = true
				}
			}
		}
		if !symbolFound {
			operandStr = " " + i.SImm16.String()
		}
	} else if i.Opcode == 1 || i.Opcode == 10 {
		// Does not print anything
	} else {
		operandStr = " " + i.SImm16.String()
	}
	s := i.InstName + operandStr
	return s
}

func (i Inst) waitcntOperandString() string {
	operandStr := ""
	if i.VMCNT != 15 {
		operandStr += fmt.Sprintf(" vmcnt(%d)", i.VMCNT)
	}

	if i.LKGMCNT != 15 {
		operandStr += fmt.Sprintf(" lgkmcnt(%d)", i.LKGMCNT)
	}
	return operandStr
}

func (i Inst) vop2String() string {
	s := fmt.Sprintf("%s %s", i.InstName, i.Dst.String())

	switch i.Opcode {
	case 25, 26, 27, 28, 29, 30:
		s += ", vcc"
	}

	s += fmt.Sprintf(", %s, %s", i.Src0.String(), i.Src1.String())

	switch i.Opcode {
	case 0, 28, 29:
		s += ", vcc"
	}

	if i.IsSdwa {
		s = strings.ReplaceAll(s, "_e32", "_sdwa")
		s += i.sdwaVOP2String()
	}

	return s
}

func (i Inst) sdwaVOP2String() string {
	s := ""

	s += " dst_sel:"
	s += sdwaSelectString(i.DstSel)
	s += " dst_unused:"
	s += sdwaUnusedString(i.DstUnused)
	s += " src0_sel:"
	s += sdwaSelectString(i.Src0Sel)
	s += " src1_sel:"
	s += sdwaSelectString(i.Src1Sel)

	return s
}

func (i Inst) vopcString() string {
	dst := "vcc"
	if strings.Contains(i.InstName, "cmpx") {
		dst = "exec"
	}

	return fmt.Sprintf("%s %s, %s, %s",
		i.InstName, dst, i.Src0.String(), i.Src1.String())
}

func (i Inst) sopcString() string {
	return fmt.Sprintf("%s %s, %s",
		i.InstName, i.Src0.String(), i.Src1.String())
}

func (i Inst) vop3aString() string {
	s := fmt.Sprintf("%s %s",
		i.InstName, i.Dst.String())

	s += ", " + i.vop3aInputOperandString(*i.Src0,
		i.isInputNeg(0),
		i.isInputAbs(0))

	s += ", " + i.vop3aInputOperandString(*i.Src1,
		i.isInputNeg(1),
		i.isInputAbs(1))

	if i.Src2 == nil {
		return s
	}

	s += ", " + i.vop3aInputOperandString(*i.Src2,
		i.isInputNeg(2),
		i.isInputAbs(2))

	return s
}

func (i Inst) vop3aInputOperandString(operand Operand, neg, abs bool) string {
	s := ""

	if neg {
		s += "-"
	}

	if abs {
		s += "|"
	}

	s += operand.String()

	if abs {
		s += "|"
	}

	return s
}

func (i Inst) isInputNeg(n uint) bool {
	mask := 1 << n
	return i.Neg&mask > 0
}

func (i Inst) isInputAbs(n uint) bool {
	mask := 1 << n
	return i.Abs&mask > 0
}

func (i Inst) vop3bString() string {
	s := i.InstName + " "

	if i.Dst != nil {
		s += i.Dst.String() + ", "
	}

	s += fmt.Sprintf("%s, %s, %s",
		i.SDst.String(),
		i.Src0.String(),
		i.Src1.String(),
	)

	if i.Opcode != 281 && i.Src2 != nil {
		s += ", " + i.Src2.String()
	}

	return s
}

func (i Inst) sop1String() string {
	return fmt.Sprintf("%s %s, %s", i.InstName, i.Dst.String(), i.Src0.String())
}

func (i Inst) sopkString() string {
	s := fmt.Sprintf("%s %s, 0x%x",
		i.InstName, i.Dst.String(), i.SImm16.IntValue)

	return s
}

func (i Inst) dsString() string {
	s := i.InstName + " "
	switch i.Opcode {
	case 54, 55, 56, 57, 58, 59, 60, 118, 119, 120:
		s += i.Dst.String() + ", "
	}

	s += i.Addr.String()

	if i.SRC0Width > 0 {
		s += ", " + i.Data.String()
	}

	if i.SRC1Width > 0 {
		s += ", " + i.Data1.String()
	}

	switch i.Opcode {
	case 13, 54:
		if i.Offset0 > 0 {
			s += fmt.Sprintf(" offset:%d", i.Offset0)
		}
	default:
		if i.Offset0 > 0 {
			s += fmt.Sprintf(" offset0:%d", i.Offset0)
		}

		if i.Offset1 > 0 {
			s += fmt.Sprintf(" offset1:%d", i.Offset1)
		}
	}

	return s
}

//nolint:gocyclo
// String returns the disassembly of an instruction
func (i Inst) String(file *elf.File) string {
	switch i.FormatType {
	case SOP2:
		return i.sop2String()
	case SMEM:
		return i.smemString()
	case VOP1:
		return i.vop1String()
	case VOP2:
		return i.vop2String()
	case FLAT:
		return i.flatString()
	case SOPP:
		return i.soppString(file)
	case VOPC:
		return i.vopcString()
	case SOPC:
		return i.sopcString()
	case VOP3a:
		return i.vop3aString()
	case VOP3b:
		return i.vop3bString()
	case SOP1:
		return i.sop1String()
	case SOPK:
		return i.sopkString()
	case DS:
		return i.dsString()
	default:
		log.Panic("Unknown instruction format type.")
		return i.InstName
	}
}
