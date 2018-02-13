package insts

import (
	"fmt"
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
// is a intruction type
type InstType struct {
	InstName string
	Opcode   Opcode
	Format   *Format
	ID       int
	ExeUnit  ExeUnit
}

// An Inst is a GCN3 instructino
type Inst struct {
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

func (i Inst) soppString() string {
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

func (i Inst) vop2String() string {
	s := fmt.Sprintf("%s %s", i.InstName, i.Dst.String())

	switch i.Opcode {
	case 25, 26, 27, 28, 29, 30:
		s += ", vcc"
	}

	s += fmt.Sprintf(", %s, %s", i.Src0.String(), i.Src1.String())

	switch i.Opcode {
	case 28, 29:
		s += ", vcc"
	}

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

func (i Inst) vop3String() string {
	// TODO: Lots of things not considered here
	s := fmt.Sprintf("%s %s, %s, %s",
		i.InstName, i.Dst.String(),
		i.Src0.String(), i.Src1.String())

	if i.Src2 != nil {
		s += ", " + i.Src2.String()
	}
	return s
}

func (i Inst) sop1String() string {
	return fmt.Sprintf("%s %s, %s", i.InstName, i.Dst.String(), i.Src0.String())
}

func (i Inst) sopkString() string {
	s := fmt.Sprintf("%s,%s,%s",
		i.InstName,i.Dst.String(),i.SImm16.String())

	return s
}
func (i Inst) String() string {
	switch i.FormatType {
	case Sop2:
		return i.sop2String()
	case Smem:
		return i.smemString()
	case Vop1:
		return i.vop1String()
	case Vop2:
		return i.vop2String()
	case Flat:
		return i.flatString()
	case Sopp:
		return i.soppString()
	case Vopc:
		return i.vopcString()
	case Sopc:
		return i.sopcString()
	case Vop3:
		return i.vop3String()
	case Sop1:
		return i.sop1String()
	case Sopk:
		return i.sopkString()
	default:
		return i.InstName
	}
}
