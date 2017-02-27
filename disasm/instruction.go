package disasm

import (
	"fmt"
	"strings"
)

// An Instruction is a GCN3 instructino
type Instruction struct {
	*Format
	*InstType
	ByteSize int

	Src0 *Operand
	Src1 *Operand
	Src2 *Operand
	Dst  *Operand

	Addr   *Operand
	Data   *Operand
	Base   *Operand
	Offset *Operand
	SImm16 *Operand

	Abs                 int
	Omod                int
	Neg                 int
	SystemLevelCoherent bool
	GlobalLevelCoherent bool
	TextureFailEnable   bool
	Imm                 bool
	Clamp               bool
}

func (i Instruction) sop2String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String() + ", " +
		i.Src1.String()
}

func (i Instruction) vop1String() string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String()
}

func (i Instruction) flatString() string {
	var s string
	if i.Opcode >= 16 && i.Opcode <= 23 {
		s = i.InstName + " " + i.Dst.String() + ", " +
			i.Addr.String()
	} else if i.Opcode >= 24 && i.Opcode <= 31 {
		s = i.InstName + " " + i.Addr.String() + ", " +
			i.Dst.String()
	}
	return s
}

func (i Instruction) smemString() string {
	// TODO: Consider store instructions, and the case if imm = 0
	s := fmt.Sprintf("%s %s, %s, %#x",
		i.InstName, i.Data.String(), i.Base.String(), uint16(i.Offset.IntValue))
	return s
}

func (i Instruction) soppString() string {
	operandStr := ""
	if i.Opcode == 12 { // S_WAITCNT
		if extractBits(uint32(i.SImm16.IntValue), 0, 3) == 0 {
			operandStr += " vmcnt(0)"
		}
		if extractBits(uint32(i.SImm16.IntValue), 8, 12) == 0 {
			operandStr += " lgkmcnt(0)"
		}
	} else if i.Opcode == 1 || i.Opcode == 10 {

	} else {
		operandStr = " " + i.SImm16.String()
	}
	s := i.InstName + operandStr
	return s
}

func (i Instruction) vop2String() string {
	s := fmt.Sprintf("%s %s, vcc, %s, %s", i.InstName,
		i.Dst.String(), i.Src0.String(), i.Src1.String())

	if i.Opcode == 28 {
		s += ", vcc"
	}

	return s
}

func (i Instruction) vopcString() string {
	dst := "vcc"
	if strings.Contains(i.InstName, "cmpx") {
		dst = "exec"
	}

	return fmt.Sprintf("%s %s, %s, %s",
		i.InstName, dst, i.Src0.String(), i.Src1.String())
}

func (i Instruction) sopcString() string {
	return fmt.Sprintf("%s %s, %s",
		i.InstName, i.Src0.String(), i.Src1.String())
}

func (i Instruction) vop3String() string {
	// TODO: Lots of things not considered here
	s := fmt.Sprintf("%s %s, %s, %s",
		i.InstName, i.Dst.String(),
		i.Src0.String(), i.Src1.String())

	if i.Src2 != nil {
		s += ", " + i.Src2.String()
	}
	return s
}

func (i Instruction) String() string {
	switch i.FormatType {
	case sop2:
		return i.sop2String()
	case smem:
		return i.smemString()
	case vop1:
		return i.vop1String()
	case vop2:
		return i.vop2String()
	case flat:
		return i.flatString()
	case sopp:
		return i.soppString()
	case vopc:
		return i.vopcString()
	case sopc:
		return i.sopcString()
	case vop3:
		return i.vop3String()
	default:
		return i.InstName
	}
}
